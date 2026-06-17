package coordinator

import (
	"context"
	"log"
	"sync"
	"time"

	"crossbow-simulation/backend/config"
	"crossbow-simulation/backend/internal/alarm_ws"
	"crossbow-simulation/backend/internal/dtu_receiver"
	"crossbow-simulation/backend/internal/fire_rate_optimizer"
	"crossbow-simulation/backend/internal/mechanism_simulator"
	"crossbow-simulation/backend/internal/middleware"
	"crossbow-simulation/backend/internal/model"
	"crossbow-simulation/backend/internal/repository"
	"crossbow-simulation/backend/internal/simulation"
)

// CrossbowInstance 单个连弩实例的所有模块
type CrossbowInstance struct {
	CrossbowID   string
	Simulator    *mechanism_simulator.MechanismSimulator
	Optimizer    *fire_rate_optimizer.FireRateOptimizer
}

// Coordinator 系统协调器
// 负责：模块生命周期管理、channel路由、数据转发、对外API接口
//
// 数据流:
// 传感器 → DTUReceiver → [校验后数据] → Coordinator → [分流]
//                                                        ├→ MechanismSimulator (对比校准)
//                                                        ├→ FireRateOptimizer (RL状态输入)
//                                                        └→ AlarmWS (告警检测)
//
// 命令流:
// API → Coordinator → [发送到对应模块]
//
// 优化流:
// FireRateOptimizer → [装填间隔调整] → Coordinator → MechanismSimulator (应用调整)
type Coordinator struct {
	// 配置
	mechParams *config.MechanismParams
	rlParams   *config.RLParams
	repo       *repository.Repository

	// 连弩实例管理
	instances map[string]*CrossbowInstance
	instancesMu sync.RWMutex

	// ===== 全局通道 =====
	// DTU接收器输入（来自API的原始传感器数据）
	dtuRawChan chan model.SensorData
	// DTU接收器输出（校验后的数据）
	dtuValidatedChan chan dtu_receiver.ValidatedData

	// ===== 各模块的channel集合（按crossbowID分组）=====
	// 模拟器命令通道
	simCmdChans map[string]chan mechanism_simulator.SimulatorCommand
	// 模拟器输出通道
	simOutputChans map[string]chan mechanism_simulator.SimulatorOutput
	// 优化器输入通道（来自协调器的状态数据）
	optInputChans map[string]chan fire_rate_optimizer.OptimizerInput
	// 优化器命令通道
	optCmdChans map[string]chan fire_rate_optimizer.OptimizerCommand
	// 优化器输出通道（装填间隔调整）
	optOutputChans map[string]chan fire_rate_optimizer.OptimizerOutput
	// 告警服务输入通道
	alarmInputChan chan alarm_ws.SensorInput

	// 模块实例
	dtuReceiver *dtu_receiver.DTUReceiver
	alarmService *alarm_ws.AlarmWS

	// 全局控制
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewCoordinator 创建协调器
func NewCoordinator(
	mechParams *config.MechanismParams,
	rlParams *config.RLParams,
	repo *repository.Repository,
) *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())

	// 全局通道
	dtuRawChan := make(chan model.SensorData, 100)
	dtuValidatedChan := make(chan dtu_receiver.ValidatedData, 100)
	alarmInputChan := make(chan alarm_ws.SensorInput, 100)

	// 创建DTU接收器
	dtuConfig := dtu_receiver.DefaultValidationConfig()
	dtuReceiver := dtu_receiver.NewDTUReceiver(dtuConfig, dtuRawChan, dtuValidatedChan)

	// 创建告警服务
	alarmConfig := alarm_ws.DefaultAlarmConfig(mechParams)
	alarmService := alarm_ws.NewAlarmWS(alarmConfig, mechParams, alarmInputChan)

	return &Coordinator{
		mechParams:      mechParams,
		rlParams:        rlParams,
		repo:            repo,
		instances:       make(map[string]*CrossbowInstance),
		dtuRawChan:      dtuRawChan,
		dtuValidatedChan: dtuValidatedChan,
		simCmdChans:     make(map[string]chan mechanism_simulator.SimulatorCommand),
		simOutputChans:  make(map[string]chan mechanism_simulator.SimulatorOutput),
		optInputChans:   make(map[string]chan fire_rate_optimizer.OptimizerInput),
		optCmdChans:     make(map[string]chan fire_rate_optimizer.OptimizerCommand),
		optOutputChans:  make(map[string]chan fire_rate_optimizer.OptimizerOutput),
		alarmInputChan:  alarmInputChan,
		dtuReceiver:     dtuReceiver,
		alarmService:    alarmService,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start 启动协调器和所有模块
func (c *Coordinator) Start() {
	log.Println("[Coordinator] Starting...")

	// 启动DTU接收器
	c.dtuReceiver.Start()

	// 启动告警服务
	c.alarmService.Start()

	// 启动协调器主循环
	c.wg.Add(1)
	go c.runLoop()

	log.Println("[Coordinator] Started")
}

// Stop 停止协调器和所有模块
func (c *Coordinator) Stop() {
	log.Println("[Coordinator] Stopping...")
	c.cancel()

	// 停止所有连弩实例
	c.instancesMu.RLock()
	for _, inst := range c.instances {
		inst.Simulator.Stop()
		inst.Optimizer.Stop()
	}
	c.instancesMu.RUnlock()

	// 停止DTU接收器
	c.dtuReceiver.Stop()

	// 停止告警服务
	c.alarmService.Stop()

	c.wg.Wait()
	log.Println("[Coordinator] Stopped")
}

// runLoop 协调器主循环
func (c *Coordinator) runLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case validated, ok := <-c.dtuValidatedChan:
			if !ok {
				return
			}
			c.handleValidatedData(validated)
		}
	}
}

// handleValidatedData 处理校验后的传感器数据
func (c *Coordinator) handleValidatedData(validated dtu_receiver.ValidatedData) {
	if !validated.IsValid {
		log.Printf("[Coordinator] Invalid data ignored: crossbow=%s, errors=%d",
			validated.Data.CrossbowID, len(validated.Errors))
		return
	}

	data := validated.Data
	crossbowID := data.CrossbowID

	// 0. Prometheus指标采集
	middleware.IncrementSensorData(crossbowID)
	middleware.SetFireRate(crossbowID, data.FireRate)
	middleware.SetStringTension(crossbowID, data.StringTension)
	middleware.SetStringFatigue(crossbowID, data.StringFatigue)
	middleware.SetBowArmDeformation(crossbowID, data.BowArmDeformation)

	// 1. 持久化到数据库
	if err := c.repo.InsertSensorData(data); err != nil {
		log.Printf("[Coordinator] Failed to persist sensor data: %v", err)
	}

	// 2. 发送到告警服务
	alarmInput := alarm_ws.SensorInput{
		CrossbowID:    crossbowID,
		SensorData:    data,
		FatigueState: model.FatigueState{
			StringFatigue: 0.0, // 可从仿真获取
		},
		IsJammingRisk: false,
	}
	select {
	case c.alarmInputChan <- alarmInput:
	default:
	}

	// 3. 如果该连弩有仿真实例在运行，发送数据做对比校准
	c.instancesMu.RLock()
	_, hasSim := c.instances[crossbowID]
	c.instancesMu.RUnlock()

	if hasSim {
		// 发送到优化器
		optInput := fire_rate_optimizer.OptimizerInput{
			CrossbowID:        crossbowID,
			FireRate:          data.FireRate,
			StringFatigue:     0.0, // 从仿真获取
			MagazineRemaining: int(data.MagazinePosition * 10),
			AverageTension:    data.StringTension,
			ShotsFired:        0,
			Timestamp:         data.Timestamp,
		}

		c.instancesMu.RLock()
		if optChan, ok := c.optInputChans[crossbowID]; ok {
			select {
			case optChan <- optInput:
			default:
			}
		}
		c.instancesMu.RUnlock()
	}

	// 4. WebSocket广播
	c.alarmService.BroadcastSensorData(data)
}

// CreateCrossbowInstance 创建连弩实例
func (c *Coordinator) CreateCrossbowInstance(crossbowID string) error {
	c.instancesMu.Lock()
	defer c.instancesMu.Unlock()

	if _, exists := c.instances[crossbowID]; exists {
		return nil // 已存在
	}

	// 创建该实例的通道
	simCmdChan := make(chan mechanism_simulator.SimulatorCommand, 10)
	simOutputChan := make(chan mechanism_simulator.SimulatorOutput, 10)
	optInputChan := make(chan fire_rate_optimizer.OptimizerInput, 10)
	optCmdChan := make(chan fire_rate_optimizer.OptimizerCommand, 10)
	optOutputChan := make(chan fire_rate_optimizer.OptimizerOutput, 10)

	c.simCmdChans[crossbowID] = simCmdChan
	c.simOutputChans[crossbowID] = simOutputChan
	c.optInputChans[crossbowID] = optInputChan
	c.optCmdChans[crossbowID] = optCmdChan
	c.optOutputChans[crossbowID] = optOutputChan

	// 创建模拟器
	simulator := mechanism_simulator.NewMechanismSimulator(
		crossbowID,
		c.mechParams,
		simCmdChan,
		simOutputChan,
	)

	// 创建优化器
	optimizer := fire_rate_optimizer.NewFireRateOptimizer(
		crossbowID,
		c.rlParams,
		optInputChan,
		optCmdChan,
		optOutputChan,
	)

	// 注册实例
	c.instances[crossbowID] = &CrossbowInstance{
		CrossbowID: crossbowID,
		Simulator:  simulator,
		Optimizer:  optimizer,
	}

	// 启动模块
	simulator.Start()
	optimizer.Start()

	// 启动仿真输出监听和优化输出监听
	c.wg.Add(2)
	go c.handleSimulatorOutput(crossbowID, simOutputChan)
	go c.handleOptimizerOutput(crossbowID, optOutputChan, simCmdChan)

	log.Printf("[Coordinator] Created crossbow instance: %s", crossbowID)
	middleware.SetSimulationsRunning(len(c.instances))
	return nil
}

// handleSimulatorOutput 监听模拟器输出
func (c *Coordinator) handleSimulatorOutput(
	crossbowID string,
	outputChan <-chan mechanism_simulator.SimulatorOutput,
) {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case output, ok := <-outputChan:
			if !ok {
				return
			}

			// 1. 发送到告警服务
			alarmInput := alarm_ws.SensorInput{
				CrossbowID:    crossbowID,
				SensorData:    output.SensorData,
				FatigueState:  model.FatigueState{
					StringFatigue: output.FatigueState.StringFatigue,
					TotalDamage:   output.FatigueState.TotalDamage,
					MaxStress:     output.FatigueState.MaxStress,
					Cycles:        output.FatigueState.Cycles,
				},
				IsJammingRisk: false,
			}
			select {
			case c.alarmInputChan <- alarmInput:
			default:
			}

			// 2. 发送到优化器
			optInput := fire_rate_optimizer.OptimizerInput{
				CrossbowID:        crossbowID,
				FireRate:          output.SensorData.FireRate,
				StringFatigue:     output.FatigueState.StringFatigue,
				MagazineRemaining: int(output.SensorData.MagazinePosition * float64(c.mechParams.Magazine.Capacity)),
				AverageTension:    output.SensorData.StringTension,
				ShotsFired:        int(output.FatigueState.Cycles),
				Timestamp:         output.Timestamp,
			}

			c.instancesMu.RLock()
			if optChan, ok := c.optInputChans[crossbowID]; ok {
				select {
				case optChan <- optInput:
				default:
				}
			}
			c.instancesMu.RUnlock()

			// 3. WebSocket广播
			c.alarmService.BroadcastDynamicsState(output.DynamicsState)
			if output.TrajectoryData != nil {
				c.alarmService.BroadcastTrajectory(output.TrajectoryData)
			}
		}
	}
}

// handleOptimizerOutput 监听优化器输出，转发到模拟器
func (c *Coordinator) handleOptimizerOutput(
	crossbowID string,
	optOutputChan <-chan fire_rate_optimizer.OptimizerOutput,
	simCmdChan chan<- mechanism_simulator.SimulatorCommand,
) {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case output, ok := <-optOutputChan:
			if !ok {
				return
			}

			// 转发调整到模拟器
			cmd := mechanism_simulator.SimulatorCommand{
				Type:           mechanism_simulator.CmdAdjustInterval,
				CrossbowID:     crossbowID,
				IntervalAdjust: output.IntervalAdjust,
			}

			select {
			case simCmdChan <- cmd:
			default:
			}

			// WebSocket广播RL状态
			_ = output
		}
	}
}

// StartSimulation 启动仿真
func (c *Coordinator) StartSimulation(crossbowID string, speedMult float64, enableRL bool) error {
	c.instancesMu.RLock()
	inst, exists := c.instances[crossbowID]
	cmdChan := c.simCmdChans[crossbowID]
	c.instancesMu.RUnlock()

	if !exists {
		// 自动创建实例
		if err := c.CreateCrossbowInstance(crossbowID); err != nil {
			return err
		}
		c.instancesMu.RLock()
		inst = c.instances[crossbowID]
		cmdChan = c.simCmdChans[crossbowID]
		c.instancesMu.RUnlock()
	}

	if inst.Simulator.IsRunning() {
		return nil // 已在运行
	}

	cmd := mechanism_simulator.SimulatorCommand{
		Type:       mechanism_simulator.CmdStart,
		CrossbowID: crossbowID,
		SpeedMult:  speedMult,
		EnableRL:   enableRL,
	}

	select {
	case cmdChan <- cmd:
	default:
		return nil
	}

	return nil
}

// StopSimulation 停止仿真
func (c *Coordinator) StopSimulation(crossbowID string) error {
	c.instancesMu.RLock()
	cmdChan, ok := c.simCmdChans[crossbowID]
	c.instancesMu.RUnlock()

	if !ok {
		return nil
	}

	cmd := mechanism_simulator.SimulatorCommand{
		Type:       mechanism_simulator.CmdStop,
		CrossbowID: crossbowID,
	}

	select {
	case cmdChan <- cmd:
	default:
	}

	return nil
}

// ResetSimulation 重置仿真
func (c *Coordinator) ResetSimulation(crossbowID string) error {
	c.instancesMu.RLock()
	cmdChan, ok := c.simCmdChans[crossbowID]
	c.instancesMu.RUnlock()

	if !ok {
		return nil
	}

	cmd := mechanism_simulator.SimulatorCommand{
		Type:       mechanism_simulator.CmdReset,
		CrossbowID: crossbowID,
	}

	select {
	case cmdChan <- cmd:
	default:
	}

	return nil
}

// StartRLTraining 启动RL训练
func (c *Coordinator) StartRLTraining(crossbowID string, magazineCapacity int) error {
	c.instancesMu.RLock()
	cmdChan, ok := c.optCmdChans[crossbowID]
	c.instancesMu.RUnlock()

	if !ok {
		return nil
	}

	cmd := fire_rate_optimizer.OptimizerCommand{
		Type:             fire_rate_optimizer.CmdStartTraining,
		CrossbowID:       crossbowID,
		MagazineCapacity: magazineCapacity,
	}

	select {
	case cmdChan <- cmd:
	default:
	}

	return nil
}

// GetRLStatus 获取RL状态
func (c *Coordinator) GetRLStatus(crossbowID string) *model.RLStatus {
	c.instancesMu.RLock()
	inst, ok := c.instances[crossbowID]
	c.instancesMu.RUnlock()

	if !ok {
		return nil
	}

	return inst.Optimizer.GetStatus()
}

// GetOptimizedPolicy 获取优化后的策略
func (c *Coordinator) GetOptimizedPolicy(crossbowID string) []float64 {
	c.instancesMu.RLock()
	inst, ok := c.instances[crossbowID]
	c.instancesMu.RUnlock()

	if !ok {
		return nil
	}

	return inst.Optimizer.GetOptimizedPolicy()
}

// PauseRLTraining 暂停RL训练
func (c *Coordinator) PauseRLTraining(crossbowID string) {
	c.instancesMu.RLock()
	inst, ok := c.instances[crossbowID]
	c.instancesMu.RUnlock()

	if ok {
		inst.Optimizer.PauseTraining()
	}
}

// ResumeRLTraining 恢复RL训练
func (c *Coordinator) ResumeRLTraining(crossbowID string) {
	c.instancesMu.RLock()
	inst, ok := c.instances[crossbowID]
	c.instancesMu.RUnlock()

	if ok {
		inst.Optimizer.ResumeTraining()
	}
}

// ReceiveSensorData 接收传感器数据（供API调用）
func (c *Coordinator) ReceiveSensorData(data model.SensorData) error {
	select {
	case c.dtuRawChan <- data:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
		// channel满，非阻塞丢弃
		log.Printf("[Coordinator] DTU channel full, dropping data: crossbow=%s", data.CrossbowID)
		return nil
	}
}

// GetAlarmService 获取告警服务
func (c *Coordinator) GetAlarmService() *alarm_ws.AlarmWS {
	return c.alarmService
}

// GetSimulatorState 获取仿真状态
func (c *Coordinator) GetSimulatorState(crossbowID string) *simulation.DynamicsState {
	c.instancesMu.RLock()
	inst, ok := c.instances[crossbowID]
	c.instancesMu.RUnlock()

	if !ok {
		return nil
	}

	state := inst.Simulator.GetState()
	return &state
}

// GetFatigueState 获取疲劳状态
func (c *Coordinator) GetFatigueState(crossbowID string) *simulation.FatigueState {
	c.instancesMu.RLock()
	inst, ok := c.instances[crossbowID]
	c.instancesMu.RUnlock()

	if !ok {
		return nil
	}

	state := inst.Simulator.GetFatigue()
	return &state
}

// IsSimulatorRunning 仿真是否运行中
func (c *Coordinator) IsSimulatorRunning(crossbowID string) bool {
	c.instancesMu.RLock()
	inst, ok := c.instances[crossbowID]
	c.instancesMu.RUnlock()

	if !ok {
		return false
	}

	return inst.Simulator.IsRunning()
}

// ListCrossbows 获取所有连弩实例
func (c *Coordinator) ListCrossbows() []string {
	c.instancesMu.RLock()
	defer c.instancesMu.RUnlock()

	ids := make([]string, 0, len(c.instances))
	for id := range c.instances {
		ids = append(ids, id)
	}
	return ids
}

// GetDTUStats 获取DTU接收器统计
func (c *Coordinator) GetDTUStats() dtu_receiver.ReceiverStats {
	return c.dtuReceiver.GetStats()
}

// GetAlarmStats 获取告警统计
func (c *Coordinator) GetAlarmStats() int64 {
	return c.alarmService.GetStats()
}

// GetClientCount 获取WebSocket客户端数量
func (c *Coordinator) GetClientCount() int {
	return c.alarmService.GetClientCount()
}

// GetAlerts 获取告警历史
func (c *Coordinator) GetAlerts(crossbowID string, limit int) []*alarm_ws.Alert {
	return c.alarmService.GetAlerts(crossbowID, limit)
}

// GetActiveAlerts 获取活跃告警
func (c *Coordinator) GetActiveAlerts(crossbowID string) []*alarm_ws.Alert {
	return c.alarmService.GetActiveAlerts(crossbowID)
}

// ResolveAlert 解决告警
func (c *Coordinator) ResolveAlert(alertID string) bool {
	return c.alarmService.ResolveAlert(alertID)
}

// RegisterWebSocketClient 注册WebSocket客户端
func (c *Coordinator) RegisterWebSocketClient(client *alarm_ws.Client) {
	c.alarmService.RegisterClient(client)
}

// UnregisterWebSocketClient 注销WebSocket客户端
func (c *Coordinator) UnregisterWebSocketClient(client *alarm_ws.Client) {
	c.alarmService.UnregisterClient(client)
}

// WritePump WebSocket写入协程
func (c *Coordinator) WritePump(client *alarm_ws.Client) {
	c.alarmService.WritePump(client)
}

// ReadPump WebSocket读取协程
func (c *Coordinator) ReadPump(client *alarm_ws.Client) {
	c.alarmService.ReadPump(client)
}
