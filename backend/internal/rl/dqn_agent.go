package rl

import (
	"math"
	"math/rand"
	"time"
)

type DQNAgent struct {
	config          TrainingConfig
	qNetwork        *QNetwork
	targetNetwork   *QNetwork
	replayBuffer    *ReplayBuffer
	epsilon         float64
	stepsSinceUpdate int
	rng             *rand.Rand
}

func NewDQNAgent(config TrainingConfig) *DQNAgent {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	stateDim := config.StateDimension
	if stateDim == 0 {
		stateDim = 5
	}

	qNet := &QNetwork{
		weights: make([]float64, stateDim),
		bias:    0.0,
	}

	targetNet := &QNetwork{
		weights: make([]float64, stateDim),
		bias:    0.0,
	}

	copy(targetNet.weights, qNet.weights)

	agent := &DQNAgent{
		config:          config,
		qNetwork:        qNet,
		targetNetwork:   targetNet,
		replayBuffer:    NewReplayBuffer(config.ReplayBufferSize),
		epsilon:         config.EpsilonStart,
		stepsSinceUpdate: 0,
		rng:             rng,
	}

	return agent
}

func NewReplayBuffer(capacity int) *ReplayBuffer {
	return &ReplayBuffer{
		buffer:   make([]Experience, capacity),
		capacity: capacity,
		head:     0,
		size:     0,
	}
}

func (rb *ReplayBuffer) Add(exp Experience) {
	rb.buffer[rb.head] = exp
	rb.head = (rb.head + 1) % rb.capacity
	if rb.size < rb.capacity {
		rb.size++
	}
}

func (rb *ReplayBuffer) Sample(batchSize int, rng *rand.Rand) []Experience {
	if rb.size < batchSize {
		batchSize = rb.size
	}

	batch := make([]Experience, batchSize)
	for i := 0; i < batchSize; i++ {
		idx := rng.Intn(rb.size)
		batch[i] = rb.buffer[idx]
	}

	return batch
}

func (rb *ReplayBuffer) Size() int {
	return rb.size
}

func (qn *QNetwork) Predict(state State) float64 {
	features := stateToFeatures(state)
	value := qn.bias
	for i, feature := range features {
		if i < len(qn.weights) {
			value += qn.weights[i] * feature
		}
	}
	return value
}

func (qn *QNetwork) PredictActionValues(state State, actionCount int) []float64 {
	values := make([]float64, actionCount)
	for action := 0; action < actionCount; action++ {
		actionState := state
		values[action] = qn.Predict(actionState) + float64(action)*0.01
	}
	return values
}

func stateToFeatures(state State) []float64 {
	return []float64{
		normalizeFireRate(state.FireRate),
		state.StringFatigue,
		normalizeMagazine(state.MagazineRemaining),
		normalizeTension(state.AverageTension),
		normalizeShotsFired(state.ShotsFired),
	}
}

func normalizeFireRate(fr float64) float64 {
	if fr <= 0 {
		return 0
	}
	return math.Min(fr/15.0, 1.0)
}

func normalizeMagazine(remaining int) float64 {
	return math.Min(float64(remaining)/30.0, 1.0)
}

func normalizeTension(tension float64) float64 {
	if tension <= 0 {
		return 0
	}
	return math.Min(tension/1500.0, 1.0)
}

func normalizeShotsFired(shots int) float64 {
	return math.Min(float64(shots)/1000.0, 1.0)
}

func (a *DQNAgent) SelectAction(state State, training bool) Action {
	if training && a.rng.Float64() < a.epsilon {
		return Action(a.rng.Intn(a.config.ActionDimension))
	}

	actionValues := a.qNetwork.PredictActionValues(state, a.config.ActionDimension)

	if state.StringFatigue >= a.config.FatigueThreshold {
		actionValues[ActionForceCooldown] += 1000.0
	}

	if state.MagazineRemaining <= 0 {
		actionValues[ActionDecreaseInterval5] -= 1000.0
		actionValues[ActionKeepInterval] -= 500.0
	}

	bestAction := 0
	bestValue := actionValues[0]
	for i := 1; i < len(actionValues); i++ {
		if actionValues[i] > bestValue {
			bestValue = actionValues[i]
			bestAction = i
		}
	}

	return Action(bestAction)
}

func (a *DQNAgent) StoreExperience(state State, action Action, reward float64, nextState State, done bool) {
	exp := Experience{
		State:     state,
		Action:    action,
		Reward:    reward,
		NextState: nextState,
		Done:      done,
		Timestamp: time.Now(),
	}
	a.replayBuffer.Add(exp)
}

func (a *DQNAgent) Train() float64 {
	if a.replayBuffer.Size() < a.config.BatchSize {
		return 0.0
	}

	batch := a.replayBuffer.Sample(a.config.BatchSize, a.rng)

	totalLoss := 0.0
	learningRate := a.config.LearningRate

	for _, exp := range batch {
		currentQ := a.qNetwork.Predict(exp.State)

		nextQValues := a.targetNetwork.PredictActionValues(exp.NextState, a.config.ActionDimension)
		maxNextQ := 0.0
		for _, v := range nextQValues {
			if v > maxNextQ {
				maxNextQ = v
			}
		}

		targetQ := exp.Reward
		if !exp.Done {
			targetQ += a.config.Gamma * maxNextQ
		}

		tdError := targetQ - currentQ
		totalLoss += tdError * tdError

		features := stateToFeatures(exp.State)
		for i := range features {
			if i < len(a.qNetwork.weights) {
				a.qNetwork.weights[i] += learningRate * tdError * features[i]
			}
		}
		a.qNetwork.bias += learningRate * tdError
	}

	a.stepsSinceUpdate++
	if a.stepsSinceUpdate >= a.config.TargetUpdateFreq {
		a.updateTargetNetwork()
		a.stepsSinceUpdate = 0
	}

	a.decayEpsilon()

	return totalLoss / float64(len(batch))
}

func (a *DQNAgent) updateTargetNetwork() {
	copy(a.targetNetwork.weights, a.qNetwork.weights)
	a.targetNetwork.bias = a.qNetwork.bias
}

func (a *DQNAgent) decayEpsilon() {
	if a.epsilon > a.config.EpsilonEnd {
		a.epsilon *= a.config.EpsilonDecay
		if a.epsilon < a.config.EpsilonEnd {
			a.epsilon = a.config.EpsilonEnd
		}
	}
}

func (a *DQNAgent) CalculateReward(state State, prevState State, fatigueGrowth float64) float64 {
	reward := state.FireRate * a.config.FireRateWeight

	reward -= fatigueGrowth * a.config.FatiguePenalty

	if state.FireRate < a.config.MinFireRate {
		deficit := a.config.MinFireRate - state.FireRate
		reward -= deficit * a.config.LowFireRatePenalty
	}

	if state.StringFatigue >= a.config.FatigueThreshold {
		reward -= 50.0
	}

	if state.MagazineRemaining <= 0 && prevState.MagazineRemaining > 0 {
		reward -= 20.0
	}

	if state.FireRate > prevState.FireRate && fatigueGrowth < 0.01 {
		reward += 10.0
	}

	return reward
}

func GetActionEffect(action Action) ActionEffect {
	switch action {
	case ActionDecreaseInterval5:
		return ActionEffect{IntervalMultiplier: 0.95, IsCooldown: false}
	case ActionKeepInterval:
		return ActionEffect{IntervalMultiplier: 1.0, IsCooldown: false}
	case ActionIncreaseInterval5:
		return ActionEffect{IntervalMultiplier: 1.05, IsCooldown: false}
	case ActionIncreaseInterval10:
		return ActionEffect{IntervalMultiplier: 1.10, IsCooldown: false}
	case ActionForceCooldown:
		return ActionEffect{IntervalMultiplier: 1.0, IsCooldown: true}
	default:
		return ActionEffect{IntervalMultiplier: 1.0, IsCooldown: false}
	}
}

func (a *DQNAgent) GetEpsilon() float64 {
	return a.epsilon
}

func (a *DQNAgent) GetWeights() []float64 {
	weights := make([]float64, len(a.qNetwork.weights))
	copy(weights, a.qNetwork.weights)
	return weights
}

func (a *DQNAgent) GetBias() float64 {
	return a.qNetwork.bias
}

func (a *DQNAgent) GetActionProbabilities(state State) []float64 {
	values := a.qNetwork.PredictActionValues(state, a.config.ActionDimension)

	expValues := make([]float64, len(values))
	sum := 0.0
	for i, v := range values {
		expValues[i] = math.Exp(v - values[0])
		sum += expValues[i]
	}

	probs := make([]float64, len(values))
	for i := range expValues {
		probs[i] = expValues[i] / sum
	}

	return probs
}

func (a *DQNAgent) Reset() {
	a.epsilon = a.config.EpsilonStart
	a.stepsSinceUpdate = 0
	a.replayBuffer = NewReplayBuffer(a.config.ReplayBufferSize)

	stateDim := a.config.StateDimension
	if stateDim == 0 {
		stateDim = 5
	}

	a.qNetwork.weights = make([]float64, stateDim)
	a.qNetwork.bias = 0.0
	a.targetNetwork.weights = make([]float64, stateDim)
	a.targetNetwork.bias = 0.0
}

// ExpertPolicy 专家策略（基于规则的启发式策略）
// 用于生成高质量演示数据，加速DQN训练收敛
type ExpertPolicy struct {
	config TrainingConfig
}

func NewExpertPolicy(config TrainingConfig) *ExpertPolicy {
	return &ExpertPolicy{
		config: config,
	}
}

// SelectAction 专家策略选择动作
// 基于以下启发式规则：
// 1. 疲劳超过阈值 → 强制冷却
// 2. 箭匣空 → 不加速
// 3. 疲劳低且射速低 → 加速
// 4. 射速接近目标 → 保持
// 5. 疲劳较高 → 减速
func (ep *ExpertPolicy) SelectAction(state State) Action {
	// 规则1: 疲劳严重 → 强制冷却
	if state.StringFatigue >= ep.config.FatigueThreshold*0.9 {
		return ActionForceCooldown
	}

	// 规则2: 箭匣空 → 保持（等待装填）
	if state.MagazineRemaining <= 0 {
		return ActionKeepInterval
	}

	// 规则3: 疲劳低且射速远低于目标 → 加速
	targetFireRate := 10.0 // 目标射速 10发/分钟
	if state.StringFatigue < ep.config.FatigueThreshold*0.3 &&
		state.FireRate < targetFireRate*0.7 {
		return ActionDecreaseInterval5
	}

	// 规则4: 疲劳低且射速偏低 → 小幅加速
	if state.StringFatigue < ep.config.FatigueThreshold*0.5 &&
		state.FireRate < targetFireRate*0.9 {
		return ActionDecreaseInterval5
	}

	// 规则5: 疲劳较高 → 减速
	if state.StringFatigue > ep.config.FatigueThreshold*0.7 {
		return ActionIncreaseInterval10
	}

	// 规则6: 射速过高 → 减速
	if state.FireRate > targetFireRate*1.1 {
		return ActionIncreaseInterval5
	}

	// 规则7: 其他情况保持
	return ActionKeepInterval
}

// GenerateDemonstrations 生成演示数据
// 使用专家策略在模拟环境中运行，收集经验
func (ep *ExpertPolicy) GenerateDemonstrations(
	numEpisodes int,
	magazineCapacity int,
) []Experience {
	demonstrations := make([]Experience, 0, numEpisodes*200)

	for epIdx := 0; epIdx < numEpisodes; epIdx++ {
		state := State{
			FireRate:          8.0,
			StringFatigue:     0.0,
			MagazineRemaining: magazineCapacity,
			AverageTension:    800.0,
			ShotsFired:        0,
		}

		loadingInterval := ep.config.BaseLoadingInterval
		isCooldown := false
		cooldownRemaining := 0
		tensionHistory := make([]float64, 0, 100)

		for step := 0; step < 200; step++ {
			action := ep.SelectAction(state)
			effect := GetActionEffect(action)

			var nextState State
			var fatigueGrowth float64

			if effect.IsCooldown {
				isCooldown = true
				cooldownRemaining = 5
				nextState = state
				nextState.StringFatigue = math.Max(0, state.StringFatigue-0.05)
				nextState.FireRate = state.FireRate * 0.5
				fatigueGrowth = 0.0
			} else if isCooldown {
				cooldownRemaining--
				if cooldownRemaining <= 0 {
					isCooldown = false
				}
				nextState = state
				nextState.StringFatigue = math.Max(0, state.StringFatigue-0.02)
				nextState.FireRate = state.FireRate * 0.8
				fatigueGrowth = 0.0
			} else {
				loadingInterval *= effect.IntervalMultiplier
				minInterval := ep.config.BaseLoadingInterval * 0.5
				maxInterval := ep.config.BaseLoadingInterval * 2.0
				if loadingInterval < minInterval {
					loadingInterval = minInterval
				}
				if loadingInterval > maxInterval {
					loadingInterval = maxInterval
				}

				nextState = state
				if nextState.MagazineRemaining > 0 {
					nextState.MagazineRemaining--
					nextState.ShotsFired++

					baseFatigueGrowth := 0.001 + (10.0/loadingInterval)*0.0005
					fatigueGrowth = baseFatigueGrowth * (1.0 + state.StringFatigue*0.5)
					nextState.StringFatigue = math.Min(0.99, state.StringFatigue+fatigueGrowth)

					nextState.FireRate = 60.0 / loadingInterval
					nextState.AverageTension = 700.0 + (10.0/loadingInterval)*100.0

					tensionHistory = append(tensionHistory, nextState.AverageTension)
					if len(tensionHistory) > 100 {
						tensionHistory = tensionHistory[1:]
					}
				} else {
					nextState.MagazineRemaining = magazineCapacity
					nextState.FireRate = state.FireRate * 0.3
					fatigueGrowth = 0.0005
					nextState.StringFatigue = math.Min(0.99, state.StringFatigue+fatigueGrowth)
				}
			}

			done := nextState.ShotsFired >= 500 || nextState.StringFatigue >= 0.95

			demonstrations = append(demonstrations, Experience{
				State:     state,
				Action:    action,
				Reward:    0.0, // 演示数据不包含奖励，用于行为克隆
				NextState: nextState,
				Done:      done,
				Timestamp: time.Now(),
			})

			state = nextState

			if done {
				break
			}
		}
	}

	return demonstrations
}

// PretrainWithDemonstrations 使用演示数据进行预训练（行为克隆）
// 通过模仿专家策略来初始化Q网络，加速后续RL训练收敛
func (a *DQNAgent) PretrainWithDemonstrations(demonstrations []Experience, epochs int) float64 {
	if len(demonstrations) == 0 {
		return 0.0
	}

	totalLoss := 0.0
	batchSize := a.config.BatchSize
	if batchSize > len(demonstrations) {
		batchSize = len(demonstrations)
	}

	learningRate := a.config.LearningRate * 3.0 // 预训练使用更大的学习率

	for epoch := 0; epoch < epochs; epoch++ {
		// 打乱演示数据
		shuffled := make([]Experience, len(demonstrations))
		copy(shuffled, demonstrations)
		for i := len(shuffled) - 1; i > 0; i-- {
			j := a.rng.Intn(i + 1)
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		}

		// 逐批次训练
		for i := 0; i < len(shuffled); i += batchSize {
			end := i + batchSize
			if end > len(shuffled) {
				end = len(shuffled)
			}
			batch := shuffled[i:end]

			batchLoss := 0.0
			for _, exp := range batch {
				// 行为克隆：预测专家动作的Q值
				// 对专家动作使用较高的目标值，其他动作使用较低值
				expertActionValue := 100.0 // 专家动作的目标Q值
				otherActionValue := -10.0  // 非专家动作的目标Q值

				actionValues := a.qNetwork.PredictActionValues(exp.State, a.config.ActionDimension)
				predictedValue := actionValues[exp.Action]

				// TD误差：向专家动作的值靠近
				tdError := expertActionValue - predictedValue
				batchLoss += tdError * tdError

				// 更新权重
				features := stateToFeatures(exp.State)
				for k := range features {
					if k < len(a.qNetwork.weights) {
						a.qNetwork.weights[k] += learningRate * tdError * features[k]
					}
				}
				a.qNetwork.bias += learningRate * tdError

				// 对非专家动作进行轻微惩罚（可选，提高动作区分度）
				for act := 0; act < a.config.ActionDimension; act++ {
					if act != int(exp.Action) {
						otherTdError := otherActionValue - actionValues[act]
						// 仅轻微更新，避免过度干扰
						weakLR := learningRate * 0.1
						for k := range features {
							if k < len(a.qNetwork.weights) {
								a.qNetwork.weights[k] += weakLR * otherTdError * features[k]
							}
						}
						a.qNetwork.bias += weakLR * otherTdError
					}
				}
			}

			totalLoss += batchLoss / float64(len(batch))
		}
	}

	// 预训练后同步目标网络
	a.updateTargetNetwork()

	// 降低探索率（因为已经有较好的初始策略）
	a.epsilon = a.config.EpsilonStart * 0.3

	return totalLoss / float64(epochs)
}

// LoadDemonstrationsIntoReplayBuffer 将演示数据加载到经验回放池
// 确保训练初期有高质量经验可供采样
func (a *DQNAgent) LoadDemonstrationsIntoReplayBuffer(demonstrations []Experience) int {
	count := 0
	for _, exp := range demonstrations {
		a.replayBuffer.Add(exp)
		count++
	}
	return count
}
