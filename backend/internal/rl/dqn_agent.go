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
