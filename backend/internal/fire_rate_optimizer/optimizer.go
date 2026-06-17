package fire_rate_optimizer

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"crossbow-simulation/backend/config"
	"crossbow-simulation/backend/internal/model"
	"crossbow-simulation/backend/internal/rl"
)

// OptimizerCommand 优化器控制命令
type OptimizerCommand struct {
	Type            CommandType
	CrossbowID      string
	MagazineCapacity int
}

type CommandType int

const (
	CmdStartTraining CommandType = iota
	CmdStopTraining
	CmdPauseTraining
	CmdResumeTraining
	CmdGetOptimizedAction
)

// OptimizerInput 优化器输入（来自协调器的状态更新）
type OptimizerInput struct {
	CrossbowID      string
	FireRate        float64
	StringFatigue   float64
	MagazineRemaining int
	AverageTension  float64
	ShotsFired      int
	Timestamp       time.Time
}

// OptimizerOutput 优化器输出（发送给协调器/模拟器）
type OptimizerOutput struct {
	CrossbowID      string
	Action          rl.Action
	IntervalAdjust  float64
	FireRate        float64
	IsTraining      bool
	Epsilon         float64
	Episode         int
	Timestamp       time.Time
}

// Status 优化器状态
type Status struct {
	IsTraining        bool
	Episode           int
	TotalReward       float64
	AverageReward     float64
	Epsilon           float64
	CurrentPolicy     []float64
	TrainingStartTime *time.Time
	BestReward        float64
	Converged         bool
}

// FireRateOptimizer 射速优化器
// 负责：强化学习训练、预训练、实时决策
// 输入：传感器状态（射速、疲劳、箭匣余量、张力）
// 输出：装填间隔调整
type FireRateOptimizer struct {
	crossbowID  string
	config      *config.RLParams
	agent       *rl.DQNAgent
	trainingCfg rl.TrainingConfig

	// 训练状态
	isTraining  bool
	isPaused    bool
	episode     int
	totalSteps  int
	bestReward  float64
	converged   bool
	trainingStartTime *time.Time
	lastUpdateTime  time.Time
	recentRewards   []float64
	currentState    rl.State
	loadingInterval float64
	isCooldown      bool
	cooldownRemaining int
	tensionHistory  []float64
	episodeReward   float64
	episodeSteps    int

	// 通信通道
	inputChan  <-chan OptimizerInput
	cmdChan    <-chan OptimizerCommand
	outputChan chan<- OptimizerOutput

	// 同步
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.RWMutex
}

// NewFireRateOptimizer 创建射速优化器
func NewFireRateOptimizer(
	crossbowID string,
	rlConfig *config.RLParams,
	inputChan <-chan OptimizerInput,
	cmdChan <-chan OptimizerCommand,
	outputChan chan<- OptimizerOutput,
) *FireRateOptimizer {
	ctx, cancel := context.WithCancel(context.Background())

	// 从JSON配置构建RL训练配置
	trainingCfg := rl.TrainingConfig{
		StateDimension:       rlConfig.Agent.StateDimension,
		ActionDimension:      rlConfig.Agent.ActionDimension,
		ReplayBufferSize:     rlConfig.Agent.ReplayBufferSize,
		BatchSize:            rlConfig.Agent.BatchSize,
		Gamma:                rlConfig.Agent.Gamma,
		EpsilonStart:         rlConfig.Agent.EpsilonStart,
		EpsilonEnd:           rlConfig.Agent.EpsilonEnd,
		EpsilonDecay:         rlConfig.Agent.EpsilonDecay,
		LearningRate:         rlConfig.Agent.LearningRate,
		TargetUpdateFreq:     rlConfig.Agent.TargetUpdateFreq,
		MaxEpisodes:          rlConfig.Training.MaxEpisodes,
		ConvergenceWindow:    rlConfig.Training.ConvergenceWindow,
		ConvergenceThreshold: rlConfig.Training.ConvergenceThreshold,
		FireRateWeight:       rlConfig.Training.FireRateWeight,
		FatiguePenalty:       rlConfig.Training.FatiguePenalty,
		LowFireRatePenalty:   rlConfig.Training.LowFireRatePenalty,
		MinFireRate:          rlConfig.Training.MinFireRate,
		FatigueThreshold:     rlConfig.Training.FatigueThreshold,
		BaseLoadingInterval:  rlConfig.Training.BaseLoadingInterval,
		PretrainEpisodes:     rlConfig.Pretrain.PretrainEpisodes,
		PretrainEpochs:       rlConfig.Pretrain.PretrainEpochs,
		EnablePretrain:       rlConfig.Pretrain.EnablePretrain,
	}

	agent := rl.NewDQNAgent(trainingCfg)

	return &FireRateOptimizer{
		crossbowID:      crossbowID,
		config:          rlConfig,
		agent:           agent,
		trainingCfg:     trainingCfg,
		isTraining:      false,
		isPaused:        false,
		loadingInterval: trainingCfg.BaseLoadingInterval,
		inputChan:       inputChan,
		cmdChan:         cmdChan,
		outputChan:      outputChan,
		ctx:             ctx,
		cancel:          cancel,
		recentRewards:   make([]float64, 0, trainingCfg.ConvergenceWindow),
		tensionHistory:  make([]float64, 0, rlConfig.ExpertPolicy.TensionHistorySize),
		bestReward:      math.Inf(-1),
	}
}

// Start 启动优化器
func (o *FireRateOptimizer) Start() {
	log.Printf("[FireRateOptimizer] Starting optimizer for crossbow=%s", o.crossbowID)
	o.wg.Add(1)
	go o.runLoop()
}

// Stop 停止优化器
func (o *FireRateOptimizer) Stop() {
	log.Printf("[FireRateOptimizer] Stopping optimizer for crossbow=%s", o.crossbowID)
	o.mu.Lock()
	o.isTraining = false
	o.mu.Unlock()
	o.cancel()
	o.wg.Wait()
	log.Printf("[FireRateOptimizer] Stopped for crossbow=%s", o.crossbowID)
}

// runLoop 主运行循环
func (o *FireRateOptimizer) runLoop() {
	defer o.wg.Done()

	for {
		select {
		case <-o.ctx.Done():
			return
		case cmd, ok := <-o.cmdChan:
			if !ok {
				return
			}
			o.handleCommand(cmd)
		case input, ok := <-o.inputChan:
			if !ok {
				return
			}
			o.processInput(input)
		}
	}
}

// handleCommand 处理控制命令
func (o *FireRateOptimizer) handleCommand(cmd OptimizerCommand) {
	o.mu.Lock()
	defer o.mu.Unlock()

	switch cmd.Type {
	case CmdStartTraining:
		o.startTraining(cmd.MagazineCapacity)
	case CmdStopTraining:
		o.isTraining = false
		log.Printf("[FireRateOptimizer] Training stopped: crossbow=%s", o.crossbowID)
	case CmdPauseTraining:
		o.isPaused = true
		log.Printf("[FireRateOptimizer] Training paused: crossbow=%s", o.crossbowID)
	case CmdResumeTraining:
		o.isPaused = false
		log.Printf("[FireRateOptimizer] Training resumed: crossbow=%s", o.crossbowID)
	case CmdGetOptimizedAction:
		// 立即输出当前最优动作
		o.outputOptimizedAction()
	}
}

// startTraining 启动训练（含预训练）
func (o *FireRateOptimizer) startTraining(magazineCapacity int) {
	now := time.Now()
	o.isTraining = true
	o.isPaused = false
	o.episode = 0
	o.totalSteps = 0
	o.bestReward = math.Inf(-1)
	o.converged = false
	o.trainingStartTime = &now
	o.lastUpdateTime = now
	o.recentRewards = make([]float64, 0, o.trainingCfg.ConvergenceWindow)
	o.episodeReward = 0
	o.episodeSteps = 0

	o.currentState = rl.State{
		FireRate:          8.0,
		StringFatigue:     0.0,
		MagazineRemaining: magazineCapacity,
		AverageTension:    800.0,
		ShotsFired:        0,
	}
	o.loadingInterval = o.trainingCfg.BaseLoadingInterval
	o.isCooldown = false
	o.cooldownRemaining = 0
	o.tensionHistory = make([]float64, 0, o.config.ExpertPolicy.TensionHistorySize)

	o.agent.Reset()

	// 预训练（模仿学习）
	if o.trainingCfg.EnablePretrain {
		o.runPretraining(magazineCapacity)
	}

	log.Printf("[FireRateOptimizer] Training started: crossbow=%s, pretrain=%v",
		o.crossbowID, o.trainingCfg.EnablePretrain)
}

// runPretraining 执行预训练
func (o *FireRateOptimizer) runPretraining(magazineCapacity int) {
	expertCfg := rl.TrainingConfig{
		StateDimension:       o.config.Agent.StateDimension,
		ActionDimension:      o.config.Agent.ActionDimension,
		ReplayBufferSize:     o.config.Agent.ReplayBufferSize,
		BatchSize:            o.config.Agent.BatchSize,
		Gamma:                o.config.Agent.Gamma,
		EpsilonStart:         o.config.Agent.EpsilonStart,
		EpsilonEnd:           o.config.Agent.EpsilonEnd,
		EpsilonDecay:         o.config.Agent.EpsilonDecay,
		LearningRate:         o.config.Agent.LearningRate,
		TargetUpdateFreq:     o.config.Agent.TargetUpdateFreq,
		MaxEpisodes:          o.config.Training.MaxEpisodes,
		ConvergenceWindow:    o.config.Training.ConvergenceWindow,
		ConvergenceThreshold: o.config.Training.ConvergenceThreshold,
		FireRateWeight:       o.config.Training.FireRateWeight,
		FatiguePenalty:       o.config.Training.FatiguePenalty,
		LowFireRatePenalty:   o.config.Training.LowFireRatePenalty,
		MinFireRate:          o.config.Training.MinFireRate,
		FatigueThreshold:     o.config.Training.FatigueThreshold,
		BaseLoadingInterval:  o.config.Training.BaseLoadingInterval,
	}

	expertPolicy := rl.NewExpertPolicy(expertCfg)
	demonstrations := expertPolicy.GenerateDemonstrations(
		o.config.Pretrain.PretrainEpisodes,
		magazineCapacity,
	)

	_ = o.agent.PretrainWithDemonstrations(demonstrations, o.config.Pretrain.PretrainEpochs)
	o.agent.LoadDemonstrationsIntoReplayBuffer(demonstrations)

	log.Printf("[FireRateOptimizer] Pretraining completed: crossbow=%s, demonstrations=%d",
		o.crossbowID, len(demonstrations))
}

// processInput 处理状态输入，选择动作并训练
func (o *FireRateOptimizer) processInput(input OptimizerInput) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// 更新当前状态
	o.currentState.FireRate = input.FireRate
	o.currentState.StringFatigue = input.StringFatigue
	o.currentState.MagazineRemaining = input.MagazineRemaining
	o.currentState.AverageTension = input.AverageTension

	o.tensionHistory = append(o.tensionHistory, input.AverageTension)
	if len(o.tensionHistory) > o.config.ExpertPolicy.TensionHistorySize {
		o.tensionHistory = o.tensionHistory[1:]
	}

	// 如果在训练中，执行一步训练
	if o.isTraining && !o.isPaused {
		action := o.agent.SelectAction(o.currentState)
		effect := rl.GetActionEffect(action)

		var nextState rl.State
		var fatigueGrowth float64

		if effect.IsCooldown {
			o.isCooldown = true
			o.cooldownRemaining = o.config.ExpertPolicy.CooldownSteps
			nextState = o.currentState
			nextState.StringFatigue = math.Max(0, o.currentState.StringFatigue-o.config.ExpertPolicy.CooldownFatigueReduction)
			nextState.FireRate = o.currentState.FireRate * 0.5
			fatigueGrowth = 0.0
		} else if o.isCooldown {
			o.cooldownRemaining--
			if o.cooldownRemaining <= 0 {
				o.isCooldown = false
			}
			nextState = o.currentState
			nextState.StringFatigue = math.Max(0, o.currentState.StringFatigue-o.config.ExpertPolicy.NonCooldownFatigueReduction)
			nextState.FireRate = o.currentState.FireRate * 0.8
			fatigueGrowth = 0.0
		} else {
			o.loadingInterval *= effect.IntervalMultiplier
			minInterval := o.trainingCfg.BaseLoadingInterval * o.config.ExpertPolicy.MinIntervalMultiplier
			maxInterval := o.trainingCfg.BaseLoadingInterval * o.config.ExpertPolicy.MaxIntervalMultiplier
			if o.loadingInterval < minInterval {
				o.loadingInterval = minInterval
			}
			if o.loadingInterval > maxInterval {
				o.loadingInterval = maxInterval
			}

			nextState = o.currentState
			if nextState.MagazineRemaining > 0 {
				nextState.MagazineRemaining--
				nextState.ShotsFired++

				baseFatigueGrowth := o.config.ExpertPolicy.BaseFatigueGrowth +
					(o.trainingCfg.BaseLoadingInterval/o.loadingInterval)*o.config.ExpertPolicy.IntervalFactorFatigueGrowth
				fatigueGrowth = baseFatigueGrowth * (1.0 + o.currentState.StringFatigue*o.config.ExpertPolicy.FatigueAccelerationFactor)
				nextState.StringFatigue = math.Min(0.99, o.currentState.StringFatigue+fatigueGrowth)

				nextState.FireRate = 60.0 / o.loadingInterval
				nextState.AverageTension = o.config.ExpertPolicy.BaseTension +
					(o.trainingCfg.BaseLoadingInterval/o.loadingInterval)*o.config.ExpertPolicy.IntervalFactorTension
			} else {
				nextState.MagazineRemaining = o.config.ExpertPolicy.TensionHistorySize
				nextState.FireRate = o.currentState.FireRate * 0.3
				fatigueGrowth = 0.0005
				nextState.StringFatigue = math.Min(0.99, o.currentState.StringFatigue+fatigueGrowth)
			}
		}

		done := nextState.ShotsFired >= 500 || nextState.StringFatigue >= 0.95

		// 计算奖励
		reward := o.agent.CalculateReward(o.currentState, nextState, fatigueGrowth)

		// 存储经验
		exp := rl.Experience{
			State:     o.currentState,
			Action:    action,
			Reward:    reward,
			NextState: nextState,
			Done:      done,
			Timestamp: time.Now(),
		}
		o.agent.StoreExperience(exp)

		// 训练一步
		_ = o.agent.TrainStep()

		// 更新统计
		o.totalSteps++
		o.episodeReward += reward
		o.episodeSteps++
		o.currentState = nextState

		// 检查episode结束
		if done {
			o.handleEpisodeEnd()
		}

		// 输出优化结果
		output := OptimizerOutput{
			CrossbowID:     o.crossbowID,
			Action:         action,
			IntervalAdjust: o.loadingInterval - o.trainingCfg.BaseLoadingInterval,
			FireRate:       nextState.FireRate,
			IsTraining:     o.isTraining && !o.isPaused,
			Epsilon:        o.agent.GetEpsilon(),
			Episode:        o.episode,
			Timestamp:      time.Now(),
		}

		select {
		case o.outputChan <- output:
		case <-o.ctx.Done():
			return
		}
	} else {
		// 不在训练中，仅用当前策略选择动作（推理模式）
		action := o.agent.SelectAction(o.currentState)
		effect := rl.GetActionEffect(action)
		intervalAdjust := (effect.IntervalMultiplier - 1.0) * o.trainingCfg.BaseLoadingInterval

		output := OptimizerOutput{
			CrossbowID:     o.crossbowID,
			Action:         action,
			IntervalAdjust: intervalAdjust,
			FireRate:       o.currentState.FireRate,
			IsTraining:     false,
			Epsilon:        o.agent.GetEpsilon(),
			Episode:        o.episode,
			Timestamp:      time.Now(),
		}

		select {
		case o.outputChan <- output:
		case <-o.ctx.Done():
			return
		}
	}
}

// handleEpisodeEnd 处理episode结束
func (o *FireRateOptimizer) handleEpisodeEnd() {
	o.episode++
	o.recentRewards = append(o.recentRewards, o.episodeReward)
	if len(o.recentRewards) > o.trainingCfg.ConvergenceWindow {
		o.recentRewards = o.recentRewards[1:]
	}

	if o.episodeReward > o.bestReward {
		o.bestReward = o.episodeReward
	}

	// 检查收敛
	if len(o.recentRewards) >= o.trainingCfg.ConvergenceWindow {
		mid := len(o.recentRewards) / 2
		firstHalf := 0.0
		secondHalf := 0.0
		for i := 0; i < mid; i++ {
			firstHalf += o.recentRewards[i]
		}
		for i := mid; i < len(o.recentRewards); i++ {
			secondHalf += o.recentRewards[i]
		}
		firstHalf /= float64(mid)
		secondHalf /= float64(mid)

		if firstHalf > 0 && math.Abs(secondHalf-firstHalf)/math.Abs(firstHalf) < o.trainingCfg.ConvergenceThreshold {
			o.converged = true
		}
	}

	o.episodeReward = 0
	o.episodeSteps = 0
	o.isCooldown = false
	o.cooldownRemaining = 0
	o.loadingInterval = o.trainingCfg.BaseLoadingInterval

	if o.episode >= o.trainingCfg.MaxEpisodes || o.converged {
		o.isTraining = false
		log.Printf("[FireRateOptimizer] Training finished: crossbow=%s, episodes=%d, converged=%v, best_reward=%.2f",
			o.crossbowID, o.episode, o.converged, o.bestReward)
	}
}

// outputOptimizedAction 输出当前最优动作
func (o *FireRateOptimizer) outputOptimizedAction() {
	action := o.agent.SelectAction(o.currentState)
	effect := rl.GetActionEffect(action)
	intervalAdjust := (effect.IntervalMultiplier - 1.0) * o.trainingCfg.BaseLoadingInterval

	output := OptimizerOutput{
		CrossbowID:     o.crossbowID,
		Action:         action,
		IntervalAdjust: intervalAdjust,
		FireRate:       o.currentState.FireRate,
		IsTraining:     o.isTraining && !o.isPaused,
		Epsilon:        o.agent.GetEpsilon(),
		Episode:        o.episode,
		Timestamp:      time.Now(),
	}

	select {
	case o.outputChan <- output:
	case <-o.ctx.Done():
	}
}

// GetStatus 获取优化器状态
func (o *FireRateOptimizer) GetStatus() *model.RLStatus {
	o.mu.RLock()
	defer o.mu.RUnlock()

	totalReward := 0.0
	avgReward := 0.0
	for _, r := range o.recentRewards {
		totalReward += r
	}
	if len(o.recentRewards) > 0 {
		avgReward = totalReward / float64(len(o.recentRewards))
	}

	return &model.RLStatus{
		IsTraining:        o.isTraining && !o.isPaused,
		Episode:           o.episode,
		TotalReward:       totalReward,
		AverageReward:     avgReward,
		Epsilon:           o.agent.GetEpsilon(),
		CurrentPolicy:     o.agent.GetWeights(),
		TrainingStartTime: o.trainingStartTime,
		BestReward:        o.bestReward,
	}
}

// GetOptimizedPolicy 获取优化后的策略权重
func (o *FireRateOptimizer) GetOptimizedPolicy() []float64 {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.agent.GetWeights()
}

// IsTraining 获取训练状态
func (o *FireRateOptimizer) IsTraining() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.isTraining && !o.isPaused
}

// PauseTraining 暂停训练
func (o *FireRateOptimizer) PauseTraining() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.isPaused = true
}

// ResumeTraining 恢复训练
func (o *FireRateOptimizer) ResumeTraining() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.isPaused = false
}
