package alert

import (
	"context"
	"log"
	"sync"
	"time"

	"crossbow-simulation/backend/internal/model"
	"crossbow-simulation/backend/internal/repository"
	"crossbow-simulation/backend/config"
)

type AlertService struct {
	repo          *repository.Repository
	wsHub         WebSocketHub
	alerts        map[string]*model.Alert
	lastAlertTime map[string]map[string]time.Time
	mu            sync.RWMutex
	checkInterval time.Duration
	cooldownPeriod time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

type WebSocketHub interface {
	BroadcastAlert(crossbowID string, alert *model.Alert)
}

func NewAlertService(repo *repository.Repository, wsHub WebSocketHub) *AlertService {
	ctx, cancel := context.WithCancel(context.Background())
	return &AlertService{
		repo:          repo,
		wsHub:         wsHub,
		alerts:        make(map[string]*model.Alert),
		lastAlertTime: make(map[string]map[string]time.Time),
		checkInterval: time.Duration(config.AppConfig.Alert.CheckInterval) * time.Second,
		cooldownPeriod: time.Duration(config.AppConfig.Alert.CooldownPeriod) * time.Second,
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (s *AlertService) Start() {
	go s.monitorLoop()
	log.Println("Alert service started")
}

func (s *AlertService) Stop() {
	s.cancel()
	log.Println("Alert service stopped")
}

func (s *AlertService) monitorLoop() {
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkAllCrossbows()
		}
	}
}

func (s *AlertService) checkAllCrossbows() {
	crossbows, _, err := s.repo.ListCrossbows(1, 100)
	if err != nil {
		log.Printf("Error listing crossbows for alert check: %v", err)
		return
	}

	for _, cb := range crossbows {
		s.checkCrossbowAlerts(cb.ID)
	}
}

func (s *AlertService) checkCrossbowAlerts(crossbowID string) {
	sensorData, err := s.repo.GetLatestSensorData(crossbowID)
	if err != nil {
		log.Printf("Error getting latest sensor data for %s: %v", crossbowID, err)
		return
	}

	thresholds, err := s.repo.GetThresholdsByCrossbowID(crossbowID)
	if err != nil {
		defaultThresholds := &model.AlertThresholds{
			CrossbowID:           crossbowID,
			StringTensionMax:     1200,
			StringFatigueWarning: 0.7,
			FireRateMin:          6,
			DeformationMax:       20,
		}
		thresholds = defaultThresholds
	}

	s.checkStringTension(crossbowID, sensorData, thresholds)
	s.checkFireRate(crossbowID, sensorData, thresholds)
	s.checkFatigue(crossbowID, sensorData, thresholds)
	s.checkDeformation(crossbowID, sensorData, thresholds)
}

func (s *AlertService) checkStringTension(crossbowID string, data *model.SensorData, thresholds *model.AlertThresholds) {
	if data.StringTension > thresholds.StringTensionMax * 0.9 {
		var level, message string
		if data.StringTension > thresholds.StringTensionMax {
			level = "critical"
			message = "弓弦张力超过断裂阈值，存在断裂风险！"
		} else if data.StringTension > thresholds.StringTensionMax * 0.95 {
			level = "danger"
			message = "弓弦张力接近断裂阈值，请注意！"
		} else {
			level = "warning"
			message = "弓弦张力偏高，建议降低射速"
		}

		s.createAlert(crossbowID, "string_break_risk", level, message,
			data.StringTension, thresholds.StringTensionMax)
	}
}

func (s *AlertService) checkFireRate(crossbowID string, data *model.SensorData, thresholds *model.AlertThresholds) {
	if data.FireRate < thresholds.FireRateMin {
		level := "warning"
		if data.FireRate < thresholds.FireRateMin * 0.7 {
			level = "danger"
		}
		message := "射速低于设计值，请检查装填机构"

		s.createAlert(crossbowID, "low_fire_rate", level, message,
			data.FireRate, thresholds.FireRateMin)
	}
}

func (s *AlertService) checkFatigue(crossbowID string, data *model.SensorData, thresholds *model.AlertThresholds) {
	if data.StringFatigue > thresholds.StringFatigueWarning {
		var level, message string
		if data.StringFatigue > 0.95 {
			level = "critical"
			message = "弓弦疲劳即将达到极限，立即停止使用！"
		} else if data.StringFatigue > 0.85 {
			level = "danger"
			message = "弓弦疲劳严重，建议更换"
		} else {
			level = "warning"
			message = "弓弦疲劳累积中，请注意监控"
		}

		s.createAlert(crossbowID, "fatigue_warning", level, message,
			data.StringFatigue, thresholds.StringFatigueWarning)
	}
}

func (s *AlertService) checkDeformation(crossbowID string, data *model.SensorData, thresholds *model.AlertThresholds) {
	if data.BowArmDeformation > thresholds.DeformationMax {
		level := "danger"
		if data.BowArmDeformation > thresholds.DeformationMax * 1.2 {
			level = "critical"
		}
		message := "弩臂变形超过安全阈值，存在失效风险！"

		s.createAlert(crossbowID, "deformation_risk", level, message,
			data.BowArmDeformation, thresholds.DeformationMax)
	}
}

func (s *AlertService) createAlert(crossbowID, alertType, level, message string, value, threshold float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isInCooldown(crossbowID, alertType) {
		return
	}

	alert := &model.Alert{
		CrossbowID: crossbowID,
		Type:       alertType,
		Level:      level,
		Message:    message,
		Value:      value,
		Threshold:  threshold,
		CreatedAt:  time.Now(),
		Acknowledged: false,
	}

	alertID, err := s.repo.CreateAlert(alert)
	if err != nil {
		log.Printf("Error creating alert: %v", err)
		return
	}
	alert.ID = alertID

	s.alerts[alertID] = alert
	s.setLastAlertTime(crossbowID, alertType, time.Now())

	if s.wsHub != nil {
		s.wsHub.BroadcastAlert(crossbowID, alert)
	}

	log.Printf("Alert created: %s - %s - %s", crossbowID, alertType, level)
}

func (s *AlertService) isInCooldown(crossbowID, alertType string) bool {
	if alertMap, ok := s.lastAlertTime[crossbowID]; ok {
		if lastTime, ok := alertMap[alertType]; ok {
			return time.Since(lastTime) < s.cooldownPeriod
		}
	}
	return false
}

func (s *AlertService) setLastAlertTime(crossbowID, alertType string, t time.Time) {
	if _, ok := s.lastAlertTime[crossbowID]; !ok {
		s.lastAlertTime[crossbowID] = make(map[string]time.Time)
	}
	s.lastAlertTime[crossbowID][alertType] = t
}

func (s *AlertService) ProcessSensorData(crossbowID string, data *model.SensorData) {
	thresholds, err := s.repo.GetThresholdsByCrossbowID(crossbowID)
	if err != nil {
		defaultThresholds := &model.AlertThresholds{
			CrossbowID:           crossbowID,
			StringTensionMax:     1200,
			StringFatigueWarning: 0.7,
			FireRateMin:          6,
			DeformationMax:       20,
		}
		thresholds = defaultThresholds
	}

	s.checkStringTension(crossbowID, data, thresholds)
	s.checkFireRate(crossbowID, data, thresholds)
	s.checkFatigue(crossbowID, data, thresholds)
	s.checkDeformation(crossbowID, data, thresholds)
}

func (s *AlertService) GetAlerts(crossbowID string, page, pageSize int) ([]model.Alert, int64, error) {
	filters := map[string]interface{}{}
	if crossbowID != "" {
		filters["crossbow_id"] = crossbowID
	}
	return s.repo.ListAlerts(filters, page, pageSize)
}

func (s *AlertService) AcknowledgeAlert(alertID string) error {
	return s.repo.AcknowledgeAlert(alertID)
}

func (s *AlertService) GetThresholds(crossbowID string) (*model.AlertThresholds, error) {
	return s.repo.GetThresholdsByCrossbowID(crossbowID)
}

func (s *AlertService) UpdateThresholds(thresholds *model.AlertThresholds) error {
	return s.repo.UpdateThresholds(thresholds)
}
