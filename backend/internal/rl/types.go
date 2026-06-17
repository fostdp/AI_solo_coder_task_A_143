package rl

import "time"

type State struct {
	FireRate         float64
	StringFatigue    float64
	MagazineRemaining int
	AverageTension   float64
	ShotsFired       int
}

type Action int

const (
	ActionDecreaseInterval5  Action = 0
	ActionKeepInterval       Action = 1
	ActionIncreaseInterval5  Action = 2
	ActionIncreaseInterval10 Action = 3
	ActionForceCooldown      Action = 4
)

type Experience struct {
	State      State
	Action     Action
	Reward     float64
	NextState  State
	Done       bool
	Timestamp  time.Time
}

type ReplayBuffer struct {
	buffer     []Experience
	capacity   int
	head       int
	size       int
}

type QNetwork struct {
	weights []float64
	bias    float64
}

type TrainingConfig struct {
	StateDimension      int
	ActionDimension     int
	ReplayBufferSize    int
	BatchSize           int
	Gamma               float64
	EpsilonStart        float64
	EpsilonEnd          float64
	EpsilonDecay        float64
	LearningRate        float64
	TargetUpdateFreq    int
	MaxEpisodes         int
	ConvergenceWindow   int
	ConvergenceThreshold float64
	FireRateWeight      float64
	FatiguePenalty      float64
	LowFireRatePenalty  float64
	MinFireRate         float64
	FatigueThreshold    float64
	BaseLoadingInterval float64
	PretrainEpisodes    int     // 预训练演示episode数
	PretrainEpochs      int     // 预训练轮数
	EnablePretrain      bool    // 是否启用预训练
}

type TrainingMetrics struct {
	Episode           int
	TotalReward       float64
	AverageReward     float64
	Epsilon           float64
	Steps             int
	Loss              float64
	FireRateHistory   []float64
	FatigueHistory    []float64
	LoadingInterval   float64
	Timestamp         time.Time
}

type TrainingState struct {
	IsTraining        bool
	IsPaused          bool
	CurrentEpisode    int
	TotalSteps        int
	Epsilon           float64
	BestReward        float64
	RecentRewards     []float64
	Converged         bool
	ConvergenceReward float64
	TrainingStartTime time.Time
	LastUpdateTime    time.Time
}

type SimulationState struct {
	CrossbowID        string
	CurrentState      State
	LoadingInterval   float64
	IsCooldown        bool
	CooldownRemaining int
	BaseInterval      float64
	TensionHistory    []float64
}

type Policy struct {
	Weights            []float64
	Bias               float64
	OptimalInterval    float64
	SustainedFireRate  float64
	AvgFatigueGrowth   float64
	ActionProbabilities []float64
}

type ActionEffect struct {
	IntervalMultiplier float64
	IsCooldown         bool
}
