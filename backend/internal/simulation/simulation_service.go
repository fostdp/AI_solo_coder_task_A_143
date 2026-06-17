package simulation

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"gonum.org/v1/gonum/mat"

	"crossbow-simulation/backend/internal/alert"
	"crossbow-simulation/backend/internal/model"
	"crossbow-simulation/backend/internal/repository"
	"crossbow-simulation/backend/internal/rl"
	"crossbow-simulation/backend/internal/websocket"
)

type SimulationService struct {
	Engine       *DynamicsEngine
	Config       SimulationConfig
	CrossbowConfig model.CrossbowConfig
	CurrentState SystemState
	StateHistory []SystemState
	ArrowFlight  []ArrowState

	IsRunning    bool
	IsPaused     bool
	StepCount    int
	StartTime    time.Time
	ElapsedTime  float64
	FireRate     float64
	CurrentCrossbowID string

	mu           sync.Mutex
	stopChan     chan bool
	arrowReleaseTime float64
	lastFireTime time.Time
	fireCount    int

	repo         *repository.Repository
	wsHub        *websocket.Hub
	rlService    *rl.RLService
	alertService *alert.AlertService
}

func DefaultConfig() SimulationConfig {
	return SimulationConfig{
		TimeStep:        0.001,
		TotalTime:     10.0,
		SaveInterval:  10,
		SpeedMultiplier: 1.0,
		Gravity:     9.81,
		AirDensity:  1.225,
	}
}

func NewSimulationService(
	simConfig SimulationConfig,
	crossbowConfig model.CrossbowConfig,
	repo *repository.Repository,
	wsHub *websocket.Hub,
	rlService *rl.RLService,
	alertService *alert.AlertService,
) (*SimulationService, error) {
	if simConfig.TimeStep <= 0 {
		simConfig = DefaultConfig()
	}

	bowArmWood := MaterialParams{
		YoungsModulus:  10e9,
		YieldStrength: 80e6,
		Density:     700,
		PoissonsRatio: 0.3,
	}

	stringMaterial := MaterialParams{
		YoungsModulus:  200e9,
		YieldStrength: 2e9,
		Density:     1100,
		PoissonsRatio: 0.3,
	}

	bowLeft := BowArmParams{
		Length:        crossbowConfig.BowArmLength,
		Width:         0.03,
		Thickness:     0.015,
		Mass:        0.8,
		MomentOfInertia: 0.015,
		PivotPoint:    *mat.NewVecDense(2, []float64{0, 0}),
		Material:      bowArmWood,
		DampingCoeff:  0.5,
	}

	bowRight := BowArmParams{
		Length:        crossbowConfig.BowArmLength,
		Width:         0.03,
		Thickness:     0.015,
		Mass:        0.8,
		MomentOfInertia: 0.015,
		PivotPoint:    *mat.NewVecDense(2, []float64{0, 0}),
		Material:      bowArmWood,
		DampingCoeff:  0.5,
	}

	bowString := BowStringParams{
		Length0:       crossbowConfig.StringLength,
		Stiffness:     crossbowConfig.BowArmStiffness,
		NonlinearCoeff: 0.1,
		MassPerUnit:   0.002,
		PreTension: crossbowConfig.StringTension,
		DampingCoeff:  0.05,
		MaxTension:    2000,
		Material:      stringMaterial,
	}

	cam := CamParams{
		BaseRadius: crossbowConfig.CamRadius,
		Lift:          crossbowConfig.CamLift,
		RotSpeed:      2.0,
		FrictionCoeff:  crossbowConfig.FrictionCoefficient,
	}

	pawl := PawlRatchetParams{
		NumTeeth: 12,
		PawlMass: 0.05,
		RatchetMass: 0.1,
		TorsionalStiffness: 1.0,
		FrictionCoeff:  crossbowConfig.FrictionCoefficient,
	}

	arrow := ArrowParams{
		Mass:        crossbowConfig.ArrowMass,
		Diameter:    0.008,
		Length:      0.3,
		DragCoeff:  0.02,
	}

	engine := NewDynamicsEngine(
		bowLeft, bowRight, bowString, cam, pawl,
		arrow, simConfig,
	)

	return &SimulationService{
		Engine:       engine,
		Config:       simConfig,
		CrossbowConfig: crossbowConfig,
		CurrentState: engine.CurrentState,
		StateHistory: make([]SystemState, 0),
		ArrowFlight:  make([]ArrowState, 0),
		stopChan:     make(chan bool),
		repo:         repo,
		wsHub:        wsHub,
		rlService:    rlService,
		alertService: alertService,
	}, nil
}

func (ss *SimulationService) Start(crossbowID string, speedMultiplier float64, enableRL bool) {
	ss.mu.Lock()
	if ss.IsRunning {
		ss.mu.Unlock()
		log.Printf("Simulation already running")
		return
	}

	ss.IsRunning = true
	ss.IsPaused = false
	ss.CurrentCrossbowID = crossbowID
	ss.StartTime = time.Now()
	ss.StepCount = 0
	ss.ElapsedTime = 0
	ss.fireCount = 0
	ss.lastFireTime = time.Now()
	ss.StateHistory = make([]SystemState, 0)
	ss.ArrowFlight = make([]ArrowState, 0)
	ss.stopChan = make(chan bool)

	if enableRL {
		go ss.rlService.StartTraining(crossbowID, ss.CrossbowConfig.MagazineCapacity)
	}

	ss.mu.Unlock()

	go ss.runLoop()
}

func (ss *SimulationService) runLoop() {
	log.Println("Starting simulation loop")

	dt := ss.Config.TimeStep
	realDt := dt / ss.Config.SpeedMultiplier
	ticker := time.NewTicker(time.Duration(realDt * float64(time.Second)))
	defer ticker.Stop()

	for {
		select {
		case <-ss.stopChan:
			log.Println("Simulation stopped")
			return
		case <-ticker.C:
		}

		ss.mu.Lock()
		if ss.IsPaused {
			ss.mu.Unlock()
			continue
		}

		t := float64(ss.StepCount) * dt
		ss.ElapsedTime = t

		phase, _ := ss.Engine.LoadingController.Update(t)
		_, pawlCmd, arrowLoad, trigger := ss.Engine.LoadingController.GetPhaseOutput(t)

		ratchetAngle := ss.CurrentState.Positions.AtVec(5)
		ratchetVel := ss.CurrentState.Velocities.AtVec(5)
		pawlAngle := ss.CurrentState.Pawl.Angle

		ss.CurrentState.Pawl = ss.Engine.UpdatePawlRatchetState(
			pawlAngle,
			ratchetAngle,
			ratchetVel,
			pawlCmd,
		)

		if trigger && ss.CurrentState.Pawl.State == RatchetFreewheeling && !ss.CurrentState.Arrow.InFlight {
			ss.releaseArrow()
			ss.fireCount++
			ss.lastFireTime = time.Now()
		}

		newState := ss.Engine.Step(dt)
		ss.CurrentState = newState

		if ss.CurrentState.Arrow.InFlight {
			ss.updateArrowFlight(dt)
		}

		ss.updateFatigue(dt)

		ss.StepCount++

		if ss.StepCount%100 == 0 {
			ss.broadcastState()
		}

		if ss.StepCount%6000 == 0 {
			ss.saveSensorData()
		}

		ss.mu.Unlock()
	}
}

func (ss *SimulationService) releaseArrow() {
	xs_dot := ss.CurrentState.Velocities.AtVec(2)
	T := ss.CurrentState.BowString.Tension
	ΔL := ss.CurrentState.BowString.Elongation

	k := ss.Engine.BowString.Stiffness
	α := ss.Engine.BowString.NonlinearCoeff
	Ep := 0.5*k*ΔL*ΔL + 0.25*α*k*ΔL*ΔL*ΔL*ΔL

	I1 := ss.Engine.BowArmLeft.MomentOfInertia
	I2 := ss.Engine.BowArmRight.MomentOfInertia
	θ1_dot := ss.CurrentState.Velocities.AtVec(0)
	θ2_dot := ss.CurrentState.Velocities.AtVec(1)
	Ek_arm := 0.5*I1*θ1_dot*θ1_dot + 0.5*I2*θ2_dot*θ2_dot

	η := ss.CrossbowConfig.StringTension / 1200.0
	if η > 0.95 {
		η = 0.95
	}
	if η < 0.6 {
		η = 0.6
	}

	E_available := η * (Ep + Ek_arm)

	m_arrow := ss.Engine.Arrow.Mass
	v0 := math.Sqrt(2 * E_available / m_arrow)

	launchAngle := 0.0

	ss.CurrentState.Arrow.Position = *mat.NewVecDense(3, []float64{0.0, 1.5, 0.0})
	ss.CurrentState.Arrow.Velocity = *mat.NewVecDense(3, []float64{
		v0 * math.Cos(launchAngle),
		v0 * math.Sin(launchAngle),
		0.0,
	})
	ss.CurrentState.Arrow.Acceleration = *mat.NewVecDense(3, nil)
	ss.CurrentState.Arrow.InFlight = true
	ss.CurrentState.Arrow.Energy = E_available
	ss.arrowReleaseTime = ss.ElapsedTime

	go ss.saveTrajectory(v0, launchAngle, 1.5)
}

func (ss *SimulationService) saveTrajectory(v0, theta, y0 float64) {
	trajectory := &model.ArrowTrajectory{
		CrossbowID:      ss.CurrentCrossbowID,
		FireTime:        time.Now(),
		InitialVelocity: v0,
		Positions:      make([]model.TrajectoryPoint, 0),
	}

	g := ss.Config.Gravity
	v0y := v0 * math.Sin(theta)
	tFlight := (v0y + math.Sqrt(v0y*v0y + 2*g*y0)) / g

	numPoints := 50
	dt := tFlight / float64(numPoints-1)

	for i := 0; i < numPoints; i++ {
		t := float64(i) * dt
		x := v0 * math.Cos(theta) * t
		y := y0 + v0*math.Sin(theta)*t - 0.5*g*t*t
		trajectory.Positions = append(trajectory.Positions, model.TrajectoryPoint{
			X: x, Y: y, Z: 0, T: t,
		})
	}

	trajectory.FlightTime = tFlight
	trajectory.ImpactPoint = trajectory.Positions[len(trajectory.Positions)-1]

	_, err := ss.repo.InsertTrajectory(trajectory)
	if err != nil {
		log.Printf("Failed to save trajectory: %v", err)
	}

	ss.wsHub.BroadcastTrajectory(ss.CurrentCrossbowID, trajectory)
}

func (ss *SimulationService) updateArrowFlight(dt float64) {
	arrow := &ss.CurrentState.Arrow
	params := ss.Engine.Arrow
	ρ_air := ss.Config.AirDensity
	g := ss.Config.Gravity
	Cd := params.DragCoeff
	d := params.Diameter
	m := params.Mass

	A := math.Pi * d * d / 4.0
	k_drag := 0.5 * ρ_air * Cd * A / m

	vx := arrow.Velocity.AtVec(0)
	vy := arrow.Velocity.AtVec(1)
	vz := arrow.Velocity.AtVec(2)

	v_mag := math.Sqrt(vx*vx + vy*vy + vz*vz)

	ax := -k_drag * vx * v_mag
	ay := -g - k_drag * vy * v_mag
	az := -k_drag * vz * v_mag

	newVx := vx + ax*dt
	newVy := vy + ay*dt
	newVz := vz + az*dt

	newX := arrow.Position.AtVec(0) + vx*dt
	newY := arrow.Position.AtVec(1) + vy*dt
	newZ := arrow.Position.AtVec(2) + vz*dt

	arrow.Position.SetVec(0, newX)
	arrow.Position.SetVec(1, newY)
	arrow.Position.SetVec(2, newZ)

	arrow.Velocity.SetVec(0, newVx)
	arrow.Velocity.SetVec(1, newVy)
	arrow.Velocity.SetVec(2, newVz)

	arrow.Acceleration.SetVec(0, ax)
	arrow.Acceleration.SetVec(1, ay)
	arrow.Acceleration.SetVec(2, az)

	newV_mag := math.Sqrt(newVx*newVx + newVy*newVy + newVz*newVz)
	arrow.Energy = 0.5 * m * newV_mag * newV_mag

	if newY <= 0 {
		arrow.Position.SetVec(1, 0)
		arrow.InFlight = false
	}
}

func (ss *SimulationService) updateFatigue(dt float64) {
	fatigue := &ss.CurrentState.Fatigue

	stressLeft := ss.CurrentState.LeftBowArm.Stress
	stressRight := ss.CurrentState.RightBowArm.Stress

	currentStress := math.Max(math.Abs(stressLeft), math.Abs(stressRight))

	if currentStress > fatigue.MaxStress {
		fatigue.MaxStress = currentStress
	}
	if currentStress < fatigue.MinStress {
		fatigue.MinStress = currentStress
	}

	if math.Abs(fatigue.MaxStress) > 1e-10 {
		fatigue.StressRatio = fatigue.MinStress / fatigue.MaxStress
	}

	stressAmplitude := (fatigue.MaxStress - fatigue.MinStress) / 2.0
	meanStress := (fatigue.MaxStress + fatigue.MinStress) / 2.0

	σ_ult := ss.Engine.BowArmLeft.Material.YieldStrength * 1.5
	var σ_aeq float64
	if σ_ult > meanStress {
		σ_aeq = stressAmplitude / (1 - meanStress/σ_ult)
	} else {
		σ_aeq = stressAmplitude
	}

	m := 5.0
	C := 1e12

	fatigueLimit := ss.Engine.BowArmLeft.Material.YieldStrength * 0.3

	if σ_aeq > fatigueLimit {
		Ni := math.Pow(C/math.Pow(σ_aeq, m), 1.0/m)

		cycleRate := 10.0
		ni := cycleRate * dt

		fatigue.DamageSum += ni / Ni
		fatigue.CycleCount += ni
	}

	fatigue.LifeFraction = fatigue.DamageSum
}

func (ss *SimulationService) broadcastState() {
	if ss.wsHub == nil {
		return
	}

	dynamicsState := model.DynamicsState{
		Timestamp:           time.Now(),
		BowArmAngle:         ss.CurrentState.Positions.AtVec(0),
		BowArmAngularVel:    ss.CurrentState.Velocities.AtVec(0),
		BowArmAngularAcc:    ss.CurrentState.Accelerations.AtVec(0),
		StringDisplacement:  ss.CurrentState.Positions.AtVec(2),
		StringVelocity:      ss.CurrentState.Velocities.AtVec(2),
		CamPosition:         ss.CurrentState.Positions.AtVec(4),
		PawlEngaged:         ss.CurrentState.Pawl.State == RatchetEngaged,
		LoadingComplete:     ss.CurrentState.Pawl.State == RatchetEngaged,
		ArrowLoaded:         ss.CurrentState.Arrow.InFlight || ss.CurrentState.Positions.AtVec(2) > 0.1,
	}

	ss.wsHub.BroadcastDynamicsState(ss.CurrentCrossbowID, &dynamicsState)
}

func (ss *SimulationService) saveSensorData() {
	if ss.repo == nil {
		return
	}

	elapsed := time.Since(ss.StartTime).Seconds()
	if elapsed > 0 {
		ss.FireRate = float64(ss.fireCount) / elapsed * 60.0
	}

	sensorData := &model.SensorData{
		CrossbowID:        ss.CurrentCrossbowID,
		Timestamp:         time.Now(),
		StringTension:     ss.CurrentState.BowString.Tension,
		BowArmDeformation: ss.CurrentState.LeftBowArm.Deflection * 1000,
		MagazinePosition:  float64(int(ss.CurrentState.Positions.AtVec(3))),
		FireRate:          ss.FireRate,
		ArrowVelocity:     math.Sqrt(
		ss.CurrentState.Arrow.Velocity.AtVec(0)*ss.CurrentState.Arrow.Velocity.AtVec(0) +
			ss.CurrentState.Arrow.Velocity.AtVec(1)*ss.CurrentState.Arrow.Velocity.AtVec(1) +
			ss.CurrentState.Arrow.Velocity.AtVec(2)*ss.CurrentState.Arrow.Velocity.AtVec(2),
		),
		CamAngle:          ss.CurrentState.Positions.AtVec(4),
		StringFatigue:     ss.CurrentState.Fatigue.LifeFraction,
		Temperature:       25.0 + ss.CurrentState.Fatigue.LifeFraction * 20,
	}

	err := ss.repo.InsertSensorData(sensorData)
	if err != nil {
		log.Printf("Failed to save sensor data: %v", err)
	}

	ss.wsHub.BroadcastSensorData(ss.CurrentCrossbowID, sensorData)
}

func (ss *SimulationService) broadcastAlert(alertType string, value, threshold float64) {
	alert := &model.Alert{
		CrossbowID: ss.CurrentCrossbowID,
		Type:     alertType,
		Level:    "warning",
		Value:    value,
		Threshold: threshold,
		Message:  fmt.Sprintf("%s warning: value=%.2f, threshold=%.2f", alertType, value, threshold),
	}

	alertLevel := "warning"
	if alertType == "string_break_risk" && value > threshold * 1.2 {
		alertLevel = "critical"
		alert.Level = "critical"
		alert.Message = fmt.Sprintf("CRITICAL: %s critical: value=%.2f, threshold=%.2f", alertType, value, threshold)
	}

	_, err := ss.repo.CreateAlert(alert)
	if err != nil {
		log.Printf("Failed to create alert: %v", err)
	}

	ss.wsHub.BroadcastAlert(ss.CurrentCrossbowID, alert)
}

func (ss *SimulationService) Pause() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.IsPaused = true
}

func (ss *SimulationService) Resume() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.IsPaused = false
}

func (ss *SimulationService) Stop(crossbowID string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.IsRunning {
		ss.IsRunning = false
		close(ss.stopChan)
		ss.rlService.StopTraining()
		log.Printf("Simulation stopped for crossbow: %s", crossbowID)
	}
}

func (ss *SimulationService) Reset(crossbowID string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	ss.Engine.initState()
	ss.CurrentState = ss.Engine.CurrentState
	ss.StateHistory = nil
	ss.ArrowFlight = nil
	ss.StepCount = 0
	ss.ElapsedTime = 0
	ss.IsRunning = false
	ss.IsPaused = false
	ss.FireRate = 0
	ss.fireCount = 0
	ss.stopChan = make(chan bool)
}

func (ss *SimulationService) GetState() SystemState {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.CurrentState
}

func (ss *SimulationService) GetFireRate() float64 {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.FireRate
}

func (ss *SimulationService) IsSimulationRunning() bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.IsRunning
}

func (ss *SimulationService) GetCurrentSensorData() *model.SensorData {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	return &model.SensorData{
		CrossbowID:        ss.CurrentCrossbowID,
		Timestamp:         time.Now(),
		StringTension:     ss.CurrentState.BowString.Tension,
		BowArmDeformation: ss.CurrentState.LeftBowArm.Deflection * 1000,
		MagazinePosition:  float64(int(ss.CurrentState.Positions.AtVec(3))),
		FireRate:          ss.FireRate,
		ArrowVelocity:     math.Sqrt(
		ss.CurrentState.Arrow.Velocity.AtVec(0)*ss.CurrentState.Arrow.Velocity.AtVec(0) +
			ss.CurrentState.Arrow.Velocity.AtVec(1)*ss.CurrentState.Arrow.Velocity.AtVec(1) +
			ss.CurrentState.Arrow.Velocity.AtVec(2)*ss.CurrentState.Arrow.Velocity.AtVec(2),
		),
		CamAngle:          ss.CurrentState.Positions.AtVec(4),
		StringFatigue:     ss.CurrentState.Fatigue.LifeFraction,
		Temperature:       25.0 + ss.CurrentState.Fatigue.LifeFraction * 20,
	}
}

func (ss *SimulationService) GetStatus() map[string]interface{} {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	return map[string]interface{}{
		"isRunning":    ss.IsRunning,
		"isPaused":     ss.IsPaused,
		"stepCount":    ss.StepCount,
		"elapsedTime":  ss.ElapsedTime,
		"fireRate":     ss.FireRate,
		"crossbowID": ss.CurrentCrossbowID,
		"stringTension":     ss.CurrentState.BowString.Tension,
		"stringFatigue": ss.CurrentState.Fatigue.LifeFraction,
		"bowArmDeflection": ss.CurrentState.LeftBowArm.Deflection * 1000,
	}
}
