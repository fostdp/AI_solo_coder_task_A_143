package alarm_ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"crossbow-simulation/backend/config"
	"crossbow-simulation/backend/internal/middleware"
	"crossbow-simulation/backend/internal/model"

	"github.com/gorilla/websocket"
)

// AlertLevel 告警级别
type AlertLevel string

const (
	AlertLevelInfo     AlertLevel = "info"
	AlertLevelWarning  AlertLevel = "warning"
	AlertLevelDanger   AlertLevel = "danger"
	AlertLevelCritical AlertLevel = "critical"
)

// AlertType 告警类型
type AlertType string

const (
	AlertTypeTension      AlertType = "string_tension"
	AlertTypeFireRate     AlertType = "fire_rate"
	AlertTypeFatigue      AlertType = "fatigue"
	AlertTypeDeformation  AlertType = "deformation"
	AlertTypeJamming      AlertType = "jamming"
)

// Alert 告警信息
type Alert struct {
	ID          string                 `json:"id"`
	CrossbowID  string                 `json:"crossbow_id"`
	Type        AlertType              `json:"type"`
	Level       AlertLevel             `json:"level"`
	Message     string                 `json:"message"`
	Value       float64                `json:"value"`
	Threshold   float64                `json:"threshold"`
	Metadata    map[string]interface{} `json:"metadata"`
	Timestamp   time.Time              `json:"timestamp"`
	Resolved    bool                   `json:"resolved"`
}

// SensorInput 传感器数据输入（来自协调器）
type SensorInput struct {
	CrossbowID     string
	SensorData     model.SensorData
	FatigueState   model.FatigueState
	IsJammingRisk  bool
}

// WebSocketMessage WebSocket消息
type WebSocketMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Client WebSocket客户端
type Client struct {
	Conn       *websocket.Conn
	Send       chan []byte
	CrossbowID string
}

// AlarmConfig 告警配置
type AlarmConfig struct {
	MaxTension       float64
	DesignFireRate   float64
	CriticalFatigue  float64
	MaxDeformation   float64
	CheckInterval    time.Duration
	CooldownPeriod   time.Duration
}

// DefaultAlarmConfig 默认告警配置（从JSON加载）
func DefaultAlarmConfig(mechParams *config.MechanismParams) AlarmConfig {
	return AlarmConfig{
		MaxTension:      mechParams.DesignSpec.MaxTension,
		DesignFireRate:  mechParams.DesignSpec.DesignFireRate,
		CriticalFatigue: mechParams.DesignSpec.CriticalFatigue,
		MaxDeformation:  mechParams.DesignSpec.MaxDeformation,
		CheckInterval:   time.Duration(config.AppConfig.Alert.CheckInterval) * time.Second,
		CooldownPeriod:  time.Duration(config.AppConfig.Alert.CooldownPeriod) * time.Second,
	}
}

// AlarmWS 告警与WebSocket服务
// 负责：告警检测、WebSocket连接管理、消息推送
type AlarmWS struct {
	config       AlarmConfig
	mechParams   *config.MechanismParams

	// 告警状态
	alerts       map[string][]*Alert
	cooldowns    map[string]time.Time
	activeAlerts map[string]*Alert

	// WebSocket连接
	clients    map[string]map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client

	// 输入通道
	inputChan  <-chan SensorInput

	// 同步
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.RWMutex

	// 统计
	alertCount int64
}

// NewAlarmWS 创建告警服务
func NewAlarmWS(
	alarmConfig AlarmConfig,
	mechParams *config.MechanismParams,
	inputChan <-chan SensorInput,
) *AlarmWS {
	ctx, cancel := context.WithCancel(context.Background())

	return &AlarmWS{
		config:       alarmConfig,
		mechParams:   mechParams,
		alerts:       make(map[string][]*Alert),
		cooldowns:    make(map[string]time.Time),
		activeAlerts: make(map[string]*Alert),
		clients:      make(map[string]map[*Client]bool),
		broadcast:    make(chan []byte, 256),
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		inputChan:    inputChan,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start 启动告警服务
func (a *AlarmWS) Start() {
	log.Println("[AlarmWS] Starting...")
	a.wg.Add(2)
	go a.runHub()
	go a.runMonitor()
}

// Stop 停止告警服务
func (a *AlarmWS) Stop() {
	log.Println("[AlarmWS] Stopping...")
	a.cancel()
	a.wg.Wait()
	log.Println("[AlarmWS] Stopped")
}

// runHub WebSocket连接管理hub
func (a *AlarmWS) runHub() {
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			return
		case client := <-a.register:
			a.mu.Lock()
			if _, ok := a.clients[client.CrossbowID]; !ok {
				a.clients[client.CrossbowID] = make(map[*Client]bool)
			}
			a.clients[client.CrossbowID][client] = true
			a.mu.Unlock()
			log.Printf("[AlarmWS] Client registered: crossbow=%s", client.CrossbowID)

		case client := <-a.unregister:
			a.mu.Lock()
			if clients, ok := a.clients[client.CrossbowID]; ok {
				if _, exists := clients[client]; exists {
					delete(clients, client)
					close(client.Send)
					if len(clients) == 0 {
						delete(a.clients, client.CrossbowID)
					}
				}
			}
			a.mu.Unlock()
			log.Printf("[AlarmWS] Client unregistered: crossbow=%s", client.CrossbowID)

		case message := <-a.broadcast:
			a.mu.RLock()
			for _, clients := range a.clients {
				for client := range clients {
					select {
					case client.Send <- message:
					default:
						close(client.Send)
						delete(clients, client)
					}
				}
			}
			a.mu.RUnlock()
		}
	}
}

// runMonitor 告警监测循环
func (a *AlarmWS) runMonitor() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			// 定时巡检（检查所有连弩的最新状态）
			a.checkAllCrossbows()
		case input, ok := <-a.inputChan:
			if !ok {
				return
			}
			// 处理传感器数据输入（即时告警检测）
			a.processSensorData(input)
		}
	}
}

// processSensorData 处理传感器数据并检测告警
func (a *AlarmWS) processSensorData(input SensorInput) {
	a.mu.Lock()
	defer a.mu.Unlock()

	crossbowID := input.CrossbowID
	data := input.SensorData

	// 1. 检测弓弦张力
	if alert := a.checkStringTension(crossbowID, data.StringTension); alert != nil {
		a.triggerAlert(alert)
	}

	// 2. 检测射速
	if alert := a.checkFireRate(crossbowID, data.FireRate); alert != nil {
		a.triggerAlert(alert)
	}

	// 3. 检测疲劳
	fatigue := input.FatigueState.StringFatigue
	if alert := a.checkFatigue(crossbowID, fatigue); alert != nil {
		a.triggerAlert(alert)
	}

	// 4. 检测弩臂变形
	if alert := a.checkDeformation(crossbowID, data.ArmDeformation); alert != nil {
		a.triggerAlert(alert)
	}

	// 5. 检测卡死风险
	if input.IsJammingRisk {
		alert := &Alert{
			ID:         generateAlertID(),
			CrossbowID: crossbowID,
			Type:       AlertTypeJamming,
			Level:      AlertLevelWarning,
			Message:    "机构卡死风险升高，凸轮压力角接近临界值",
			Value:      1.0,
			Threshold:  0.5,
			Timestamp:  time.Now(),
		}
		a.triggerAlert(alert)
	}
}

// checkStringTension 检测弓弦张力
func (a *AlarmWS) checkStringTension(crossbowID string, tension float64) *Alert {
	maxTension := a.config.MaxTension
	key := crossbowID + "_" + string(AlertTypeTension)

	if tension >= maxTension*0.95 {
		if !a.isInCooldown(key) {
			a.setCooldown(key)
			return &Alert{
				ID:         generateAlertID(),
				CrossbowID: crossbowID,
				Type:       AlertTypeTension,
				Level:      AlertLevelCritical,
				Message:    "弓弦张力严重超限，断裂风险极高！",
				Value:      tension,
				Threshold:  maxTension,
				Timestamp:  time.Now(),
			}
		}
	} else if tension >= maxTension*0.9 {
		if !a.isInCooldown(key) {
			a.setCooldown(key)
			return &Alert{
				ID:         generateAlertID(),
				CrossbowID: crossbowID,
				Type:       AlertTypeTension,
				Level:      AlertLevelDanger,
				Message:    "弓弦张力接近安全极限，建议立即冷却",
				Value:      tension,
				Threshold:  maxTension,
				Timestamp:  time.Now(),
			}
		}
	} else if tension >= maxTension*0.8 {
		if !a.isInCooldown(key) {
			a.setCooldown(key)
			return &Alert{
				ID:         generateAlertID(),
				CrossbowID: crossbowID,
				Type:       AlertTypeTension,
				Level:      AlertLevelWarning,
				Message:    "弓弦张力较高，注意监控",
				Value:      tension,
				Threshold:  maxTension * 0.8,
				Timestamp:  time.Now(),
			}
		}
	}

	return nil
}

// checkFireRate 检测射速
func (a *AlarmWS) checkFireRate(crossbowID string, fireRate float64) *Alert {
	designRate := a.config.DesignFireRate
	key := crossbowID + "_" + string(AlertTypeFireRate)

	if fireRate < designRate*0.5 {
		if !a.isInCooldown(key) {
			a.setCooldown(key)
			return &Alert{
				ID:         generateAlertID(),
				CrossbowID: crossbowID,
				Type:       AlertTypeFireRate,
				Level:      AlertLevelDanger,
				Message:    "射速严重低于设计值，请检查装填机构",
				Value:      fireRate,
				Threshold:  designRate,
				Timestamp:  time.Now(),
			}
		}
	} else if fireRate < designRate*0.7 {
		if !a.isInCooldown(key) {
			a.setCooldown(key)
			return &Alert{
				ID:         generateAlertID(),
				CrossbowID: crossbowID,
				Type:       AlertTypeFireRate,
				Level:      AlertLevelWarning,
				Message:    "射速低于设计值，建议检查润滑",
				Value:      fireRate,
				Threshold:  designRate,
				Timestamp:  time.Now(),
			}
		}
	}

	return nil
}

// checkFatigue 检测疲劳
func (a *AlarmWS) checkFatigue(crossbowID string, fatigue float64) *Alert {
	critical := a.config.CriticalFatigue
	key := crossbowID + "_" + string(AlertTypeFatigue)

	if fatigue >= critical*0.95 {
		if !a.isInCooldown(key) {
			a.setCooldown(key)
			return &Alert{
				ID:         generateAlertID(),
				CrossbowID: crossbowID,
				Type:       AlertTypeFatigue,
				Level:      AlertLevelCritical,
				Message:    "弓弦接近使用寿命，建议立即更换！",
				Value:      fatigue,
				Threshold:  critical,
				Timestamp:  time.Now(),
			}
		}
	} else if fatigue >= critical*0.85 {
		if !a.isInCooldown(key) {
			a.setCooldown(key)
			return &Alert{
				ID:         generateAlertID(),
				CrossbowID: crossbowID,
				Type:       AlertTypeFatigue,
				Level:      AlertLevelDanger,
				Message:    "弓弦疲劳严重，建议安排更换",
				Value:      fatigue,
				Threshold:  critical,
				Timestamp:  time.Now(),
			}
		}
	} else if fatigue >= critical*0.7 {
		if !a.isInCooldown(key) {
			a.setCooldown(key)
			return &Alert{
				ID:         generateAlertID(),
				CrossbowID: crossbowID,
				Type:       AlertTypeFatigue,
				Level:      AlertLevelWarning,
				Message:    "弓弦疲劳累积，注意监控",
				Value:      fatigue,
				Threshold:  critical * 0.7,
				Timestamp:  time.Now(),
			}
		}
	}

	return nil
}

// checkDeformation 检测弩臂变形
func (a *AlarmWS) checkDeformation(crossbowID string, deformation float64) *Alert {
	maxDef := a.config.MaxDeformation
	key := crossbowID + "_" + string(AlertTypeDeformation)

	absDef := deformation
	if absDef < 0 {
		absDef = -absDef
	}

	if absDef >= maxDef*1.5 {
		if !a.isInCooldown(key) {
			a.setCooldown(key)
			return &Alert{
				ID:         generateAlertID(),
				CrossbowID: crossbowID,
				Type:       AlertTypeDeformation,
				Level:      AlertLevelCritical,
				Message:    "弩臂严重超限，存在断裂风险！",
				Value:      absDef,
				Threshold:  maxDef,
				Timestamp:  time.Now(),
			}
		}
	} else if absDef >= maxDef*1.2 {
		if !a.isInCooldown(key) {
			a.setCooldown(key)
			return &Alert{
				ID:         generateAlertID(),
				CrossbowID: crossbowID,
				Type:       AlertTypeDeformation,
				Level:      AlertLevelDanger,
				Message:    "弩臂变形超过设计值",
				Value:      absDef,
				Threshold:  maxDef,
				Timestamp:  time.Now(),
			}
		}
	}

	return nil
}

// triggerAlert 触发告警（保存+推送）
func (a *AlarmWS) triggerAlert(alert *Alert) {
	a.alertCount++

	// Prometheus指标
	middleware.IncrementAlert(alert.CrossbowID, string(alert.Level))

	// 保存到历史
	if _, ok := a.alerts[alert.CrossbowID]; !ok {
		a.alerts[alert.CrossbowID] = make([]*Alert, 0, 100)
	}
	a.alerts[alert.CrossbowID] = append(a.alerts[alert.CrossbowID], alert)
	if len(a.alerts[alert.CrossbowID]) > 1000 {
		a.alerts[alert.CrossbowID] = a.alerts[alert.CrossbowID][1:]
	}

	// 更新活跃告警
	activeKey := alert.CrossbowID + "_" + string(alert.Type)
	a.activeAlerts[activeKey] = alert

	// 推送WebSocket
	a.broadcastAlert(alert)

	log.Printf("[AlarmWS] Alert triggered: crossbow=%s, type=%s, level=%s, msg=%s",
		alert.CrossbowID, alert.Type, alert.Level, alert.Message)
}

// broadcastAlert 广播告警到WebSocket
func (a *AlarmWS) broadcastAlert(alert *Alert) {
	msg := WebSocketMessage{
		Type:    "alert",
		Payload: alert,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[AlarmWS] Failed to marshal alert: %v", err)
		return
	}

	select {
	case a.broadcast <- data:
	case <-a.ctx.Done():
	}
}

// BroadcastSensorData 广播传感器数据
func (a *AlarmWS) BroadcastSensorData(data model.SensorData) {
	msg := WebSocketMessage{
		Type:    "sensor_data",
		Payload: data,
	}

	dataBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[AlarmWS] Failed to marshal sensor data: %v", err)
		return
	}

	select {
	case a.broadcast <- dataBytes:
	case <-a.ctx.Done():
	}
}

// BroadcastDynamicsState 广播动力学状态
func (a *AlarmWS) BroadcastDynamicsState(state interface{}) {
	msg := WebSocketMessage{
		Type:    "dynamics_state",
		Payload: state,
	}

	dataBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[AlarmWS] Failed to marshal dynamics state: %v", err)
		return
	}

	select {
	case a.broadcast <- dataBytes:
	case <-a.ctx.Done():
	}
}

// BroadcastTrajectory 广播弹道数据
func (a *AlarmWS) BroadcastTrajectory(traj interface{}) {
	msg := WebSocketMessage{
		Type:    "trajectory",
		Payload: traj,
	}

	dataBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[AlarmWS] Failed to marshal trajectory: %v", err)
		return
	}

	select {
	case a.broadcast <- dataBytes:
	case <-a.ctx.Done():
	}
}

// checkAllCrossbows 定时巡检所有连弩
func (a *AlarmWS) checkAllCrossbows() {
	// 这里可以从数据库查询所有连弩的最新数据
	// 目前通过processSensorData即时处理
}

// isInCooldown 检查是否在冷却期
func (a *AlarmWS) isInCooldown(key string) bool {
	if lastTime, ok := a.cooldowns[key]; ok {
		if time.Since(lastTime) < a.config.CooldownPeriod {
			return true
		}
	}
	return false
}

// setCooldown 设置冷却
func (a *AlarmWS) setCooldown(key string) {
	a.cooldowns[key] = time.Now()
}

// RegisterClient 注册WebSocket客户端
func (a *AlarmWS) RegisterClient(client *Client) {
	select {
	case a.register <- client:
	case <-a.ctx.Done():
	}
}

// UnregisterClient 注销WebSocket客户端
func (a *AlarmWS) UnregisterClient(client *Client) {
	select {
	case a.unregister <- client:
	case <-a.ctx.Done():
	}
}

// GetAlerts 获取连弩的告警历史
func (a *AlarmWS) GetAlerts(crossbowID string, limit int) []*Alert {
	a.mu.RLock()
	defer a.mu.RUnlock()

	alerts, ok := a.alerts[crossbowID]
	if !ok {
		return []*Alert{}
	}

	if limit <= 0 || limit > len(alerts) {
		limit = len(alerts)
	}

	// 返回最新的limit条
	result := make([]*Alert, limit)
	start := len(alerts) - limit
	copy(result, alerts[start:])
	return result
}

// GetActiveAlerts 获取活跃告警
func (a *AlarmWS) GetActiveAlerts(crossbowID string) []*Alert {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]*Alert, 0)
	prefix := crossbowID + "_"
	for key, alert := range a.activeAlerts {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, alert)
		}
	}
	return result
}

// ResolveAlert 标记告警已解决
func (a *AlarmWS) ResolveAlert(alertID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, alerts := range a.alerts {
		for _, alert := range alerts {
			if alert.ID == alertID {
				alert.Resolved = true
				// 从活跃告警中移除
				activeKey := alert.CrossbowID + "_" + string(alert.Type)
				if active, ok := a.activeAlerts[activeKey]; ok && active.ID == alertID {
					delete(a.activeAlerts, activeKey)
				}
				return true
			}
		}
	}
	return false
}

// GetStats 获取统计信息
func (a *AlarmWS) GetStats() int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.alertCount
}

// GetClientCount 获取连接的客户端数量
func (a *AlarmWS) GetClientCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	count := 0
	for _, clients := range a.clients {
		count += len(clients)
	}
	return count
}

// generateAlertID 生成告警ID
func generateAlertID() string {
	return "alert_" + time.Now().Format("20060102150405") + "_" + randomString(6)
}

// randomString 生成随机字符串
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

// WritePump 客户端写入协程
func (a *AlarmWS) WritePump(client *Client) {
	ticker := time.NewTicker(time.Duration(config.AppConfig.WebSocket.PingInterval) * time.Second)
	defer ticker.Stop()
	defer client.Conn.Close()

	for {
		select {
		case message, ok := <-client.Send:
			if !ok {
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ReadPump 客户端读取协程
func (a *AlarmWS) ReadPump(client *Client) {
	defer func() {
		a.UnregisterClient(client)
		client.Conn.Close()
	}()

	client.Conn.SetReadLimit(512)
	client.Conn.SetReadDeadline(time.Now().Add(time.Duration(config.AppConfig.WebSocket.PongTimeout) * time.Second))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(time.Duration(config.AppConfig.WebSocket.PongTimeout) * time.Second))
		return nil
	})

	for {
		_, _, err := client.Conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
