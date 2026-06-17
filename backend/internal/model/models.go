package model

import (
	"encoding/json"
	"time"
)

type CrossbowConfig struct {
	BowArmLength        float64 `json:"bowArmLength"`
	BowArmStiffness     float64 `json:"bowArmStiffness"`
	StringLength        float64 `json:"stringLength"`
	StringTension       float64 `json:"stringTension"`
	StringFatigueLimit  float64 `json:"stringFatigueLimit"`
	ArrowMass           float64 `json:"arrowMass"`
	MagazineCapacity    int     `json:"magazineCapacity"`
	CamRadius           float64 `json:"camRadius"`
	CamLift             float64 `json:"camLift"`
	FrictionCoefficient float64 `json:"frictionCoefficient"`
	Gravity             float64 `json:"gravity"`
}

type Crossbow struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Status      string          `json:"status"`
	Config      CrossbowConfig  `json:"config"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type SensorData struct {
	CrossbowID        string    `json:"crossbowId"`
	Timestamp         time.Time `json:"timestamp"`
	StringTension     float64   `json:"stringTension"`
	BowArmDeformation float64   `json:"bowArmDeformation"`
	MagazinePosition  float64   `json:"magazinePosition"`
	FireRate          float64   `json:"fireRate"`
	ArrowVelocity     float64   `json:"arrowVelocity"`
	CamAngle          float64   `json:"camAngle"`
	StringFatigue     float64   `json:"stringFatigue"`
	Temperature       float64   `json:"temperature"`
}

type DynamicsState struct {
	Timestamp           time.Time       `json:"timestamp"`
	BowArmAngle         float64         `json:"bowArmAngle"`
	BowArmAngularVel    float64         `json:"bowArmAngularVel"`
	BowArmAngularAcc    float64         `json:"bowArmAngularAcc"`
	StringDisplacement  float64         `json:"stringDisplacement"`
	StringVelocity      float64         `json:"stringVelocity"`
	CamPosition         float64         `json:"camPosition"`
	PawlEngaged         bool            `json:"pawlEngaged"`
	LoadingComplete     bool            `json:"loadingComplete"`
	ArrowLoaded         bool            `json:"arrowLoaded"`
	Forces              json.RawMessage `json:"forces,omitempty"`
}

type TrajectoryPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
	T float64 `json:"t"`
}

type ArrowTrajectory struct {
	ID              string            `json:"id"`
	CrossbowID      string            `json:"crossbowId"`
	FireTime        time.Time         `json:"fireTime"`
	Positions       []TrajectoryPoint `json:"positions"`
	InitialVelocity float64           `json:"initialVelocity"`
	FlightTime      float64           `json:"flightTime"`
	ImpactPoint     TrajectoryPoint   `json:"impactPoint"`
	CreatedAt       time.Time         `json:"createdAt"`
}

type Alert struct {
	ID             string    `json:"id"`
	CrossbowID     string    `json:"crossbowId"`
	Type           string    `json:"type"`
	Level          string    `json:"level"`
	Message        string    `json:"message"`
	Value          float64   `json:"value"`
	Threshold      float64   `json:"threshold"`
	CreatedAt      time.Time `json:"createdAt"`
	Acknowledged   bool      `json:"acknowledged"`
	AcknowledgedAt time.Time `json:"acknowledgedAt,omitempty"`
}

type AlertThresholds struct {
	ID                  string    `json:"id"`
	CrossbowID          string    `json:"crossbowId"`
	StringTensionMax    float64   `json:"stringTensionMax"`
	StringFatigueWarning float64  `json:"stringFatigueWarning"`
	FireRateMin         float64   `json:"fireRateMin"`
	DeformationMax      float64   `json:"deformationMax"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// FatigueState 疲劳状态（与simulation.FatigueState兼容）
type FatigueState struct {
	StringFatigue       float64     `json:"stringFatigue"`
	TotalDamage         float64     `json:"totalDamage"`
	MaxStress           float64     `json:"maxStress"`
	Cycles              float64     `json:"cycles"`
	TotalDeltaL         float64     `json:"totalDeltaL"`
	CurrentLifeFraction float64     `json:"currentLifeFraction"`
	TensionHistory      []float64   `json:"tensionHistory,omitempty"`
	LastUpdated         time.Time   `json:"lastUpdated"`
}

type RLStatus struct {
	IsTraining        bool      `json:"isTraining"`
	Episode           int       `json:"episode"`
	TotalReward       float64   `json:"totalReward"`
	AverageReward     float64   `json:"averageReward"`
	Epsilon           float64   `json:"epsilon"`
	CurrentPolicy     []float64 `json:"currentPolicy"`
	TrainingStartTime time.Time `json:"trainingStartTime,omitempty"`
	BestReward        float64   `json:"bestReward"`
}

type RLResult struct {
	ID                         string    `json:"id"`
	CrossbowID                 string    `json:"crossbowId"`
	OptimizedFireRate          float64   `json:"optimizedFireRate"`
	OptimizedLoadingInterval   float64   `json:"optimizedLoadingInterval"`
	FatigueReduction           float64   `json:"fatigueReduction"`
	EfficiencyImprovement      float64   `json:"efficiencyImprovement"`
	SustainedFireDuration      float64   `json:"sustainedFireDuration"`
	TrainingEpisodes           int       `json:"trainingEpisodes"`
	ConvergenceReward          float64   `json:"convergenceReward"`
	FinalPolicy                []float64 `json:"finalPolicy"`
	CreatedAt                  time.Time `json:"createdAt"`
}

type RLTrainingRecord struct {
	ID            string    `json:"id"`
	CrossbowID    string    `json:"crossbowId"`
	Episode       int       `json:"episode"`
	TotalReward   float64   `json:"totalReward"`
	AverageReward float64   `json:"averageReward"`
	Epsilon       float64   `json:"epsilon"`
	Policy        []float64 `json:"policy"`
	CreatedAt     time.Time `json:"createdAt"`
}

type WSMessage struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp string      `json:"timestamp"`
}

type StartSimulationRequest struct {
	SimulationSpeed float64 `json:"simulationSpeed"`
	EnableRL        bool    `json:"enableRL"`
	Duration        int     `json:"duration"`
}

type DataQueryRequest struct {
	CrossbowID  string   `json:"crossbowId"`
	StartTime   string   `json:"startTime"`
	EndTime     string   `json:"endTime"`
	Metrics     []string `json:"metrics"`
	Aggregation string   `json:"aggregation"`
	Interval    string   `json:"interval"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func DefaultCrossbowConfig() CrossbowConfig {
	return CrossbowConfig{
		BowArmLength:        0.65,
		BowArmStiffness:     5000,
		StringLength:        0.85,
		StringTension:       950,
		StringFatigueLimit:  0.9,
		ArrowMass:           0.05,
		MagazineCapacity:    10,
		CamRadius:           0.04,
		CamLift:             0.12,
		FrictionCoefficient: 0.15,
		Gravity:             9.81,
	}
}
