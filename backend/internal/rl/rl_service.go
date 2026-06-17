package rl

import (
	"math"
	"sync"
	"time"

	"crossbow-simulation/backend/internal/model"
)

type RLService struct {
	agent           *DQNAgent
	config          TrainingConfig
	trainingState   *TrainingState
	simulationState *SimulationState
	metricsHistory  []TrainingMetrics
	mu              sync.RWMutex
	onMetricsUpdate func(metrics TrainingMetrics)
	onTrainingDone  func(result *Policy)
}

func DefaultTrainingConfig() TrainingConfig {
	return TrainingConfig{
		StateDimension:       5,
		ActionDimension:      5,
		ReplayBufferSize:     10000,
		BatchSize:            64,
		Gamma:                0.99,
		EpsilonStart:         1.0,
		EpsilonEnd:           0.01,
		EpsilonDecay:         0.995,
		LearningRate:         0.001,
		TargetUpdateFreq:     100,
		MaxEpisodes:          1000,
		ConvergenceWindow:    100,
		ConvergenceThreshold: 0.01,
		FireRateWeight:       10.0,
		FatiguePenalty:       100.0,
		LowFireRatePenalty:   50.0,
		MinFireRate:          6.0,
		FatigueThreshold:     0.8,
		BaseLoadingInterval:  5.0,
		PretrainEpisodes:     50,
		PretrainEpochs:       20,
		EnablePretrain:       true,
	}
}

func NewRLService(config TrainingConfig) *RLService {
	if config.StateDimension == 0 {
		config = DefaultTrainingConfig()
	}

	return &RLService{
		agent:         NewDQNAgent(config),
		config:        config,
		trainingState: &TrainingState{},
		simulationState: &SimulationState{
			LoadingInterval: config.BaseLoadingInterval,
			BaseInterval:    config.BaseLoadingInterval,
			TensionHistory:  make([]float64, 0, 100),
		},
		metricsHistory: make([]TrainingMetrics, 0, config.MaxEpisodes),
	}
}

func (s *RLService) StartTraining(crossbowID string, magazineCapacity int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.simulationState.CrossbowID = crossbowID
	s.simulationState.CurrentState = State{
		FireRate:          8.0,
		StringFatigue:     0.0,
		MagazineRemaining: magazineCapacity,
		AverageTension:    800.0,
		ShotsFired:        0,
	}
	s.simulationState.LoadingInterval = s.config.BaseLoadingInterval
	s.simulationState.IsCooldown = false
	s.simulationState.CooldownRemaining = 0
	s.simulationState.TensionHistory = make([]float64, 0, 100)

	s.trainingState = &TrainingState{
		IsTraining:        true,
		IsPaused:          false,
		CurrentEpisode:    0,
		TotalSteps:        0,
		Epsilon:           s.config.EpsilonStart,
		BestReward:        math.Inf(-1),
		RecentRewards:     make([]float64, 0, s.config.ConvergenceWindow),
		Converged:         false,
		TrainingStartTime: time.Now(),
		LastUpdateTime:    time.Now(),
	}

	s.metricsHistory = make([]TrainingMetrics, 0, s.config.MaxEpisodes)
	s.agent.Reset()

	// 如果启用预训练，则先执行预训练
	if s.config.EnablePretrain {
		s.runPretraining(magazineCapacity)
	}
}

// runPretraining 执行预训练（模仿学习）
func (s *RLService) runPretraining(magazineCapacity int) {
	// 1. 生成专家演示数据
	expertPolicy := NewExpertPolicy(s.config)
	demonstrations := expertPolicy.GenerateDemonstrations(
		s.config.PretrainEpisodes,
		magazineCapacity,
	)

	// 2. 使用演示数据进行行为克隆预训练
	_ = s.agent.PretrainWithDemonstrations(demonstrations, s.config.PretrainEpochs)

	// 3. 将演示数据加载到经验回放池
	s.agent.LoadDemonstrationsIntoReplayBuffer(demonstrations)

	// 更新epsilon（预训练后降低探索率）
	s.trainingState.Epsilon = s.agent.GetEpsilon()
}

func (s *RLService) RunTrainingEpisode(magazineCapacity int) TrainingMetrics {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.trainingState.IsTraining || s.trainingState.IsPaused {
		return TrainingMetrics{}
	}

	episode := s.trainingState.CurrentEpisode + 1
	steps := 0
	totalReward := 0.0
	totalLoss := 0.0
	trainSteps := 0

	prevState := s.simulationState.CurrentState
	state := prevState

	fireRateHistory := make([]float64, 0)
	fatigueHistory := make([]float64, 0)

	maxSteps := 200
	done := false

	for steps < maxSteps && !done {
		action := s.agent.SelectAction(state, true)
		effect := GetActionEffect(action)

		nextState, fatigueGrowth := s.simulateStep(state, action, effect, magazineCapacity)
		reward := s.agent.CalculateReward(nextState, state, fatigueGrowth)

		if nextState.ShotsFired >= 500 || nextState.StringFatigue >= 0.95 {
			done = true
		}

		s.agent.StoreExperience(state, action, reward, nextState, done)

		if s.agent.replayBuffer.Size() >= s.config.BatchSize {
			loss := s.agent.Train()
			totalLoss += loss
			trainSteps++
		}

		state = nextState
		totalReward += reward
		steps++

		fireRateHistory = append(fireRateHistory, state.FireRate)
		fatigueHistory = append(fatigueHistory, state.StringFatigue)
	}

	avgLoss := 0.0
	if trainSteps > 0 {
		avgLoss = totalLoss / float64(trainSteps)
	}

	avgReward := totalReward / float64(steps)

	s.trainingState.CurrentEpisode = episode
	s.trainingState.TotalSteps += steps
	s.trainingState.Epsilon = s.agent.GetEpsilon()
	s.trainingState.LastUpdateTime = time.Now()

	if totalReward > s.trainingState.BestReward {
		s.trainingState.BestReward = totalReward
	}

	s.trainingState.RecentRewards = append(s.trainingState.RecentRewards, totalReward)
	if len(s.trainingState.RecentRewards) > s.config.ConvergenceWindow {
		s.trainingState.RecentRewards = s.trainingState.RecentRewards[1:]
	}

	metrics := TrainingMetrics{
		Episode:         episode,
		TotalReward:     totalReward,
		AverageReward:   avgReward,
		Epsilon:         s.trainingState.Epsilon,
		Steps:           steps,
		Loss:            avgLoss,
		FireRateHistory: fireRateHistory,
		FatigueHistory:  fatigueHistory,
		LoadingInterval: s.simulationState.LoadingInterval,
		Timestamp:       time.Now(),
	}

	s.metricsHistory = append(s.metricsHistory, metrics)

	if len(s.trainingState.RecentRewards) >= s.config.ConvergenceWindow {
		if s.checkConvergence() {
			s.trainingState.Converged = true
			s.trainingState.ConvergenceReward = s.computeRecentAvgReward()
			s.trainingState.IsTraining = false

			if s.onTrainingDone != nil {
				result := s.GetOptimizedPolicy()
				s.onTrainingDone(result)
			}
		}
	}

	if episode >= s.config.MaxEpisodes {
		s.trainingState.IsTraining = false
		if s.onTrainingDone != nil {
			result := s.GetOptimizedPolicy()
			s.onTrainingDone(result)
		}
	}

	if s.onMetricsUpdate != nil {
		s.onMetricsUpdate(metrics)
	}

	return metrics
}

func (s *RLService) simulateStep(state State, action Action, effect ActionEffect, magazineCapacity int) (State, float64) {
	nextState := state

	if effect.IsCooldown {
		s.simulationState.IsCooldown = true
		s.simulationState.CooldownRemaining = 5
		nextState.StringFatigue = math.Max(0, state.StringFatigue-0.05)
		nextState.FireRate = state.FireRate * 0.5
		fatigueGrowth := 0.0
		return nextState, fatigueGrowth
	}

	if s.simulationState.IsCooldown {
		s.simulationState.CooldownRemaining--
		if s.simulationState.CooldownRemaining <= 0 {
			s.simulationState.IsCooldown = false
		}
		nextState.StringFatigue = math.Max(0, state.StringFatigue-0.02)
		nextState.FireRate = state.FireRate * 0.8
		return nextState, 0.0
	}

	s.simulationState.LoadingInterval *= effect.IntervalMultiplier
	minInterval := s.config.BaseLoadingInterval * 0.5
	maxInterval := s.config.BaseLoadingInterval * 2.0
	if s.simulationState.LoadingInterval < minInterval {
		s.simulationState.LoadingInterval = minInterval
	}
	if s.simulationState.LoadingInterval > maxInterval {
		s.simulationState.LoadingInterval = maxInterval
	}

	if nextState.MagazineRemaining > 0 {
		nextState.MagazineRemaining--
		nextState.ShotsFired++

		baseFatigueGrowth := 0.001 + (10.0/s.simulationState.LoadingInterval)*0.0005
		fatigueGrowth := baseFatigueGrowth * (1.0 + state.StringFatigue*0.5)
		nextState.StringFatigue = math.Min(0.99, state.StringFatigue+fatigueGrowth)

		nextState.FireRate = 60.0 / s.simulationState.LoadingInterval
		nextState.AverageTension = 700.0 + (10.0/s.simulationState.LoadingInterval)*100.0

		s.simulationState.TensionHistory = append(s.simulationState.TensionHistory, nextState.AverageTension)
		if len(s.simulationState.TensionHistory) > 100 {
			s.simulationState.TensionHistory = s.simulationState.TensionHistory[1:]
		}

		if len(s.simulationState.TensionHistory) > 0 {
			sum := 0.0
			for _, t := range s.simulationState.TensionHistory {
				sum += t
			}
			nextState.AverageTension = sum / float64(len(s.simulationState.TensionHistory))
		}

		return nextState, fatigueGrowth
	}

	nextState.MagazineRemaining = magazineCapacity
	nextState.FireRate = state.FireRate * 0.3
	fatigueGrowth := 0.0005
	nextState.StringFatigue = math.Min(0.99, state.StringFatigue+fatigueGrowth)

	return nextState, fatigueGrowth
}

func (s *RLService) checkConvergence() bool {
	window := s.trainingState.RecentRewards
	if len(window) < s.config.ConvergenceWindow {
		return false
	}

	half := len(window) / 2
	firstHalf := window[:half]
	secondHalf := window[half:]

	avgFirst := s.average(firstHalf)
	avgSecond := s.average(secondHalf)

	if avgFirst == 0 {
		return false
	}

	change := math.Abs(avgSecond-avgFirst) / math.Abs(avgFirst)
	return change < s.config.ConvergenceThreshold
}

func (s *RLService) average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func (s *RLService) computeRecentAvgReward() float64 {
	return s.average(s.trainingState.RecentRewards)
}

func (s *RLService) GetOptimizedPolicy() *Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	optimalState := State{
		FireRate:          10.0,
		StringFatigue:     0.3,
		MagazineRemaining: 20,
		AverageTension:    850.0,
		ShotsFired:        100,
	}

	actionProbs := s.agent.GetActionProbabilities(optimalState)

	bestAction := 0
	bestProb := 0.0
	for i, prob := range actionProbs {
		if prob > bestProb {
			bestProb = prob
			bestAction = i
		}
	}

	effect := GetActionEffect(Action(bestAction))
	optimalInterval := s.config.BaseLoadingInterval * effect.IntervalMultiplier

	sustainedFireRate := 60.0 / optimalInterval
	if sustainedFireRate > 12.0 {
		sustainedFireRate = 12.0
	}

	return &Policy{
		Weights:             s.agent.GetWeights(),
		Bias:                s.agent.GetBias(),
		OptimalInterval:     optimalInterval,
		SustainedFireRate:   sustainedFireRate,
		AvgFatigueGrowth:    0.0008 * (10.0 / optimalInterval),
		ActionProbabilities: actionProbs,
	}
}

func (s *RLService) PredictAction(state State) Action {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agent.SelectAction(state, false)
}

func (s *RLService) GetTrainingStatus() *TrainingState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := *s.trainingState
	if len(s.trainingState.RecentRewards) > 0 {
		status.BestReward = s.trainingState.BestReward
	}
	return &status
}

func (s *RLService) GetLatestMetrics() *TrainingMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.metricsHistory) == 0 {
		return nil
	}

	latest := s.metricsHistory[len(s.metricsHistory)-1]
	return &latest
}

func (s *RLService) GetMetricsHistory() []TrainingMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := make([]TrainingMetrics, len(s.metricsHistory))
	copy(history, s.metricsHistory)
	return history
}

func (s *RLService) PauseTraining() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trainingState.IsPaused = true
}

func (s *RLService) ResumeTraining() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trainingState.IsPaused = false
}

func (s *RLService) StopTraining() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trainingState.IsTraining = false
}

func (s *RLService) IsTraining() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trainingState.IsTraining && !s.trainingState.IsPaused
}

func (s *RLService) IsConverged() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trainingState.Converged
}

func (s *RLService) GetStatus() *model.RLStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	avgReward := 0.0
	if len(s.trainingState.RecentRewards) > 0 {
		sum := 0.0
		for _, r := range s.trainingState.RecentRewards {
			sum += r
		}
		avgReward = sum / float64(len(s.trainingState.RecentRewards))
	}

	totalReward := 0.0
	if len(s.metricsHistory) > 0 {
		for _, m := range s.metricsHistory {
			totalReward += m.TotalReward
		}
	}

	return &model.RLStatus{
		IsTraining:        s.trainingState.IsTraining && !s.trainingState.IsPaused,
		Episode:             s.trainingState.CurrentEpisode,
		TotalReward:         totalReward,
		AverageReward:       avgReward,
		Epsilon:             s.trainingState.Epsilon,
		CurrentPolicy:       s.agent.GetWeights(),
		TrainingStartTime: s.trainingState.TrainingStartTime,
		BestReward:          s.trainingState.BestReward,
	}
}

func (s *RLService) SetOnMetricsUpdate(callback func(metrics TrainingMetrics)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onMetricsUpdate = callback
}

func (s *RLService) SetOnTrainingDone(callback func(result *Policy)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTrainingDone = callback
}

func (s *RLService) UpdateSimulationState(sensorData State, magazineCapacity int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.simulationState.CurrentState = sensorData

	if sensorData.MagazineRemaining <= 0 {
		s.simulationState.CurrentState.MagazineRemaining = magazineCapacity
	}

	if sensorData.StringFatigue >= s.config.FatigueThreshold {
		s.simulationState.IsCooldown = true
		s.simulationState.CooldownRemaining = 3
	}
}

func (s *RLService) GetCurrentLoadingInterval() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.simulationState.LoadingInterval
}

func (s *RLService) GetConfig() TrainingConfig {
	return s.config
}

func (s *RLService) EvaluatePolicy(episodes int, magazineCapacity int) (avgReward float64, avgFireRate float64, avgFatigue float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	totalReward := 0.0
	totalFireRate := 0.0
	totalFatigue := 0.0

	originalState := s.simulationState.CurrentState
	originalInterval := s.simulationState.LoadingInterval

	for i := 0; i < episodes; i++ {
		state := State{
			FireRate:          8.0,
			StringFatigue:     0.0,
			MagazineRemaining: magazineCapacity,
			AverageTension:    800.0,
			ShotsFired:        0,
		}

		episodeReward := 0.0
		episodeFireRate := 0.0
		episodeFatigue := 0.0
		steps := 0

		for steps < 100 && state.StringFatigue < 0.9 {
			action := s.agent.SelectAction(state, false)
			effect := GetActionEffect(action)

			nextState, fatigueGrowth := s.simulateStep(state, action, effect, magazineCapacity)
			reward := s.agent.CalculateReward(nextState, state, fatigueGrowth)

			episodeReward += reward
			episodeFireRate += nextState.FireRate
			episodeFatigue += nextState.StringFatigue

			state = nextState
			steps++
		}

		totalReward += episodeReward / float64(steps)
		totalFireRate += episodeFireRate / float64(steps)
		totalFatigue += episodeFatigue / float64(steps)
	}

	s.simulationState.CurrentState = originalState
	s.simulationState.LoadingInterval = originalInterval

	return totalReward / float64(episodes),
		totalFireRate / float64(episodes),
		totalFatigue / float64(episodes)
}
