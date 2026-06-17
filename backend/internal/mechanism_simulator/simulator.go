package mechanism_simulator

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"crossbow-simulation/backend/config"
	"crossbow-simulation/backend/internal/model"
	"crossbow-simulation/backend/internal/simulation"
)

// SimulatorCommand 模拟器控制命令
type SimulatorCommand struct {
	Type          CommandType
	CrossbowID    string
	SpeedMult     float64
	EnableRL      bool
	IntervalAdjust float64 // RL优化后的装填间隔调整
}

type CommandType int

const (
	CmdStart CommandType = iota
	CmdStop
	CmdReset
	CmdAdjustInterval
)

// SimulatorOutput 模拟器输出（发送到协调器）
type SimulatorOutput struct {
	CrossbowID     string
	SensorData     model.SensorData
	DynamicsState  simulation.DynamicsState
	TrajectoryData *simulation.TrajectoryData
	FatigueState   simulation.FatigueState
	Phase          int
	Timestamp      time.Time
}

// MechanismSimulator 机构动力学模拟器
// 负责：多刚体动力学计算、凸轮机构运动学、装填击发时序、疲劳累积、弹道计算
// 输入：控制命令、RL优化的装填间隔
// 输出：传感器数据、动力学状态、弹道数据、疲劳状态
type MechanismSimulator struct {
	crossbowID string

	// 配置参数（从JSON加载）
	mechParams *config.MechanismParams

	// 动力学引擎
	engine     *simulation.DynamicsEngine
	camMech    *simulation.CamMechanism
	loadingCtrl *simulation.AutoLoadingController

	// 当前状态
	state       *simulation.DynamicsState
	fatigue     simulation.FatigueState
	isRunning   bool
	stepCount   int
	speedMult   float64
	enableRL    bool
	customInterval float64 // RL优化后的装填间隔

	// 通信通道
	inputChan  <-chan SimulatorCommand
	outputChan chan<- SimulatorOutput

	// 同步控制
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.RWMutex

	// 内部缓存
	lastSensorTime time.Time
	lastSaveTime   time.Time
}

// NewMechanismSimulator 创建机构动力学模拟器
func NewMechanismSimulator(
	crossbowID string,
	mechParams *config.MechanismParams,
	inputChan <-chan SimulatorCommand,
	outputChan chan<- SimulatorOutput,
) *MechanismSimulator {
	ctx, cancel := context.WithCancel(context.Background())

	// 从配置创建凸轮机构
	camParams := simulation.CamParams{
		BaseRadius:    mechParams.Cam.BaseRadius,
		RollerRadius:  mechParams.Cam.RollerRadius,
		PressureAngle: mechParams.Cam.PressureAngle,
		Lift:          mechParams.Cam.Lift,
		RotationSpeed: mechParams.Cam.RotationSpeed,
		PhaseAngle:    mechParams.Cam.PhaseAngle,
		FrictionCoeff: mechParams.Cam.FrictionCoeff,
		RotSpeed:      mechParams.Cam.RotSpeed,
	}

	springParams := simulation.BufferSpringParams{
		Stiffness:      mechParams.BufferSpring.Stiffness,
		Preload:        mechParams.BufferSpring.Preload,
		Damping:        mechParams.BufferSpring.Damping,
		MaxCompression: mechParams.BufferSpring.MaxCompression,
		EquivalentMass: mechParams.BufferSpring.EquivalentMass,
	}

	// 创建动力学引擎
	bowArm := simulation.BowArmParams{
		Length:       mechParams.BowArm.Length,
		Width:        mechParams.BowArm.Width,
		Thickness:    mechParams.BowArm.Thickness,
		YoungsModulus: mechParams.BowArm.YoungsModulus,
		MaxStress:    mechParams.BowArm.MaxStress,
		PreTension:   mechParams.BowString.PreTension,
		DampingCoeff: mechParams.BowArm.DampingCoeff,
		Material:     "wood",
	}

	bowString := simulation.BowStringParams{
		Length0:       mechParams.BowString.Length0,
		Radius:        mechParams.BowString.Radius,
		YoungsModulus: mechParams.BowString.YoungsModulus,
		YieldStrength: mechParams.BowString.YieldStrength,
		PreTension:    mechParams.BowString.PreTension,
		DampingCoeff:  mechParams.BowString.DampingCoeff,
		Material:      mechParams.BowString.Material,
	}

	pawlRatchet := simulation.PawlRatchetParams{
		NumTeeth:      mechParams.PawlRatchet.NumTeeth,
		PawlMass:      mechParams.PawlRatchet.PawlMass,
		PawlLength:    mechParams.PawlRatchet.PawlLength,
		SpringStiffness: mechParams.PawlRatchet.SpringStiffness,
		Damping:       mechParams.PawlRatchet.Damping,
		FrictionCoeff: mechParams.PawlRatchet.FrictionCoeff,
	}

	engine := simulation.NewDynamicsEngine(bowArm, bowString, pawlRatchet)
	engine.Gravity = mechParams.Simulation.Gravity
	engine.AirDensity = mechParams.Simulation.AirDensity

	camMech := simulation.NewCamMechanism(camParams, springParams)
	camMech.FollowerMass = mechParams.BufferSpring.EquivalentMass
	camMech.GenerateProfile(360)

	loadingCtrl := simulation.NewAutoLoadingController(camMech)

	return &MechanismSimulator{
		crossbowID:     crossbowID,
		mechParams:     mechParams,
		engine:         engine,
		camMech:        camMech,
		loadingCtrl:    loadingCtrl,
		isRunning:      false,
		speedMult:      mechParams.Simulation.SpeedMultiplier,
		customInterval: 0,
		inputChan:      inputChan,
		outputChan:     outputChan,
		ctx:            ctx,
		cancel:         cancel,
		state:          simulation.NewDynamicsState(),
		lastSensorTime: time.Now(),
		lastSaveTime:   time.Now(),
	}
}

// Start 启动模拟器
func (s *MechanismSimulator) Start() {
	log.Printf("[MechanismSimulator] Starting simulator for crossbow=%s", s.crossbowID)
	s.wg.Add(1)
	go s.runLoop()
}

// Stop 停止模拟器
func (s *MechanismSimulator) Stop() {
	log.Printf("[MechanismSimulator] Stopping simulator for crossbow=%s", s.crossbowID)
	s.mu.Lock()
	s.isRunning = false
	s.mu.Unlock()
	s.cancel()
	s.wg.Wait()
	log.Printf("[MechanismSimulator] Stopped for crossbow=%s", s.crossbowID)
}

// runLoop 主运行循环
func (s *MechanismSimulator) runLoop() {
	defer s.wg.Done()

	dt := s.mechParams.Simulation.Dt
	t := 0.0

	for {
		select {
		case <-s.ctx.Done():
			return
		case cmd, ok := <-s.inputChan:
			if !ok {
				return
			}
			s.handleCommand(cmd)
		default:
			s.mu.RLock()
			running := s.isRunning
			speedMult := s.speedMult
			s.mu.RUnlock()

			if !running {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			// 执行仿真步
			s.step(dt, t)
			t += dt

			// 输出传感器数据（每1秒或100步）
			s.mu.RLock()
			stepCount := s.stepCount
			s.mu.RUnlock()

			if stepCount%100 == 0 {
				s.broadcastState(t)
			}

			// 保存到数据库（每6000步 = 30秒仿真时间）
			if stepCount%6000 == 0 {
				s.saveSensorData(t)
			}

			// 控制实时速度
			sleepTime := time.Duration(float64(time.Millisecond) * dt * 1000 / speedMult)
			if sleepTime > 0 {
				time.Sleep(sleepTime)
			}
		}
	}
}

// handleCommand 处理控制命令
func (s *MechanismSimulator) handleCommand(cmd SimulatorCommand) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch cmd.Type {
	case CmdStart:
		s.isRunning = true
		if cmd.SpeedMult > 0 {
			s.speedMult = cmd.SpeedMult
		}
		s.enableRL = cmd.EnableRL
		log.Printf("[MechanismSimulator] Started: crossbow=%s, speed=%.1fx, RL=%v",
			s.crossbowID, s.speedMult, s.enableRL)

	case CmdStop:
		s.isRunning = false
		log.Printf("[MechanismSimulator] Stopped: crossbow=%s", s.crossbowID)

	case CmdReset:
		s.resetState()
		log.Printf("[MechanismSimulator] Reset: crossbow=%s", s.crossbowID)

	case CmdAdjustInterval:
		s.customInterval = cmd.IntervalAdjust
		log.Printf("[MechanismSimulator] Adjusted interval: crossbow=%s, adjust=%.4f",
			s.crossbowID, s.customInterval)
	}
}

// resetState 重置仿真状态
func (s *MechanismSimulator) resetState() {
	s.state = simulation.NewDynamicsState()
	s.fatigue = simulation.FatigueState{}
	s.stepCount = 0
	s.lastSensorTime = time.Now()
	s.lastSaveTime = time.Now()
	s.customInterval = 0
	s.loadingCtrl.CurrentPhase = 0
	s.loadingCtrl.PhaseStartTime = 0
	for i := range s.loadingCtrl.Sequence {
		s.loadingCtrl.Sequence[i].Completed = false
	}
}

// step 执行一步仿真
func (s *MechanismSimulator) step(dt float64, t float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stepCount++

	// 1. 更新装填时序
	phase, _ := s.loadingCtrl.Update(t)
	camTarget, pawlCmd, arrowLoad, trigger := s.loadingCtrl.GetPhaseOutput(t)

	// 2. RL调整：如果有自定义间隔，调整凸轮转速
	effectiveRotSpeed := s.mechParams.Cam.RotationSpeed
	if s.enableRL && s.customInterval != 0 {
		baseInterval := s.mechParams.Training.BaseLoadingInterval
		effectiveInterval := baseInterval + s.customInterval
		if effectiveInterval > 0 {
			effectiveRotSpeed = s.mechParams.Cam.RotationSpeed * (baseInterval / effectiveInterval)
		}
	}

	// 3. 凸轮机构更新（带弹簧缓冲）
	s.camMech.CurrentAngle += effectiveRotSpeed * dt
	idealFollower := s.camMech.CalculateIdealFollowerMotion(s.camMech.CurrentAngle, effectiveRotSpeed)
	prevCamFollower := s.state.CamFollower
	newCamFollower := s.camMech.UpdateFollowerWithSpring(prevCamFollower, s.camMech.CurrentAngle, effectiveRotSpeed, dt)
	s.state.CamFollower = newCamFollower

	// 4. 动力学引擎积分
	_ = camTarget
	_ = pawlCmd
	_ = arrowLoad

	if trigger && s.state.GeneralizedCoords[2] < -0.05 {
		// 触发发射
		simConfig := simulation.DefaultSimulationConfig()
		arrowParams := simulation.ArrowParams{
			Mass:      s.mechParams.Arrow.Mass,
			Length:    s.mechParams.Arrow.Length,
			Radius:    s.mechParams.Arrow.Radius,
			DragCoeff: s.mechParams.Arrow.DragCoeff,
		}

		trajData, fireRate := s.calculateTrajectory(simConfig, arrowParams, t)
		_ = fireRate

		if trajData != nil {
			output := SimulatorOutput{
				CrossbowID:     s.crossbowID,
				TrajectoryData: trajData,
				Timestamp:      time.Now(),
			}
			select {
			case s.outputChan <- output:
			case <-s.ctx.Done():
			}
		}

		// 更新疲劳
		s.updateFatigue(s.state.GeneralizedCoords[2])

		// 重置弓弦位移
		s.state.GeneralizedCoords[2] = 0
		s.state.GeneralizedVelocities[2] = 0
	}

	// 5. RK4积分（正常流程）
	if !trigger {
		s.state = s.engine.RK4Step(*s.state, dt)
	}

	// 6. 检查凸轮卡死风险
	if s.camMech.CheckJamming(newCamFollower) {
		log.Printf("[MechanismSimulator] WARNING: Jamming risk detected for crossbow=%s, pressure_angle=%.3f",
			s.crossbowID, newCamFollower.PressureAngle)
	}

	_ = phase
}

// calculateTrajectory 计算弹道
func (s *MechanismSimulator) calculateTrajectory(
	simConfig simulation.SimulationConfig,
	arrowParams simulation.ArrowParams,
	t float64,
) (*simulation.TrajectoryData, float64) {
	tension := s.engine.CalculateBowStringTension(s.state.GeneralizedCoords)

	releaseTime := t
	initialVel := math.Sqrt(2 * tension * math.Abs(s.state.GeneralizedCoords[2]) / arrowParams.Mass)
	launchAngle := 10.0 * math.Pi / 180.0

	velVec := *simulation.NewVec3D(
		initialVel*math.Cos(launchAngle),
		initialVel*math.Sin(launchAngle),
		0.0,
	)
	posVec := *simulation.NewVec3D(0.0, 0.0, 0.0)

	trajectory := s.engine.CalculateTrajectory(posVec, velVec, arrowParams, simConfig)

	fireRate := s.state.GeneralizedCoords[2]
	if math.Abs(fireRate) > 1e-6 {
		fireRate = 1.0 / math.Abs(fireRate)
	} else {
		fireRate = 0
	}

	return &simulation.TrajectoryData{
		ReleaseTime:     releaseTime,
		InitialVelocity: velVec,
		LaunchAngle:     launchAngle,
		Points:          trajectory,
		ImpactTime:      s.state.GeneralizedCoords[0],
		ImpactVelocity:  velVec,
		MaxHeight:       s.state.GeneralizedCoords[1],
		FlightTime:      s.state.GeneralizedCoords[2],
		Range:           s.state.GeneralizedCoords[3],
		Tension:         tension,
		Energy:          0.5 * arrowParams.Mass * initialVel * initialVel,
	}, fireRate
}

// updateFatigue 更新疲劳累积
func (s *MechanismSimulator) updateFatigue(stringDisplacement float64) {
	tension := s.engine.CalculateBowStringTension(s.state.GeneralizedCoords)
	deltaL := math.Abs(stringDisplacement)
	stress := tension * s.mechParams.BowString.YoungsModulus / s.mechParams.BowString.Length0 * s.mechParams.BowString.Length0

	stressAmplitude := stress / 2.0
	stressMean := stress / 2.0

	uts := s.mechParams.BowString.YieldStrength
	goodmanCorrectedStress := stressAmplitude / (1.0 - stressMean/uts)

	cyclesToFailure := math.Pow(
		s.mechParams.BowString.FatigueStrengthCoeff/goodmanCorrectedStress,
		1.0/s.mechParams.BowString.FatigueStrengthExponent,
	)

	damage := 1.0 / cyclesToFailure
	s.fatigue.TotalDamage += damage
	s.fatigue.Cycles++
	s.fatigue.MaxStress = math.Max(s.fatigue.MaxStress, stress)
	s.fatigue.CurrentLifeFraction = s.fatigue.TotalDamage

	if s.fatigue.TensionHistory == nil {
		s.fatigue.TensionHistory = make([]float64, 0, 1000)
	}
	s.fatigue.TensionHistory = append(s.fatigue.TensionHistory, tension)
	if len(s.fatigue.TensionHistory) > 1000 {
		s.fatigue.TensionHistory = s.fatigue.TensionHistory[1:]
	}

	s.fatigue.StringFatigue = math.Min(0.99, s.fatigue.TotalDamage)
	s.fatigue.LastUpdated = time.Now()
	s.fatigue.TotalDeltaL += deltaL
}

// broadcastState 广播动力学状态
func (s *MechanismSimulator) broadcastState(t float64) {
	s.mu.RLock()
	state := *s.state
	fatigue := s.fatigue
	s.mu.RUnlock()

	tension := s.engine.CalculateBowStringTension(&state)
	deformation := state.BowArmState.Deflection
	magazinePos := float64(state.GeneralizedCoords[4]) / s.mechParams.Cam.Lift
	fireRate := 1.0 / math.Max(0.001, t)

	sensorData := model.SensorData{
		CrossbowID:      s.crossbowID,
		StringTension:   tension,
		ArmDeformation:  deformation,
		MagazinePosition: magazinePos,
		FireRate:        fireRate,
		Timestamp:       time.Now(),
	}

	output := SimulatorOutput{
		CrossbowID:    s.crossbowID,
		SensorData:    sensorData,
		DynamicsState: state,
		FatigueState:  fatigue,
		Phase:         s.loadingCtrl.CurrentPhase,
		Timestamp:     time.Now(),
	}

	select {
	case s.outputChan <- output:
	case <-s.ctx.Done():
	}
}

// saveSensorData 保存传感器数据（标记为需要持久化）
func (s *MechanismSimulator) saveSensorData(t float64) {
	s.broadcastState(t)
}

// GetState 获取当前状态
func (s *MechanismSimulator) GetState() simulation.DynamicsState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.state
}

// GetFatigue 获取疲劳状态
func (s *MechanismSimulator) GetFatigue() simulation.FatigueState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fatigue
}

// IsRunning 获取运行状态
func (s *MechanismSimulator) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// GetStepCount 获取步数
func (s *MechanismSimulator) GetStepCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stepCount
}
