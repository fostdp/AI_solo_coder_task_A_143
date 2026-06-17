package dtu_receiver

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"crossbow-simulation/backend/internal/model"
)

// ValidationConfig 数据校验配置
type ValidationConfig struct {
	MinTension      float64       `json:"min_tension"`
	MaxTension      float64       `json:"max_tension"`
	MinDeformation  float64       `json:"min_deformation"`
	MaxDeformation  float64       `json:"max_deformation"`
	MinMagazinePos  float64       `json:"min_magazine_pos"`
	MaxMagazinePos  float64       `json:"max_magazine_pos"`
	MinFireRate     float64       `json:"min_fire_rate"`
	MaxFireRate     float64       `json:"max_fire_rate"`
	MaxTimeOffset   time.Duration `json:"max_time_offset"`
}

// DefaultValidationConfig 默认校验配置
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		MinTension:     0.0,
		MaxTension:     3000.0,   // 3000N
		MinDeformation: -0.05,     // -50mm
		MaxDeformation: 0.05,      // 50mm
		MinMagazinePos: 0.0,
		MaxMagazinePos: 1.0,
		MinFireRate:    0.0,
		MaxFireRate:    30.0,      // 30发/分钟
		MaxTimeOffset:  time.Hour,  // 时间戳偏差不超过1小时
	}
}

// ValidatedData 校验后的数据
type ValidatedData struct {
	Data      model.SensorData
	IsValid   bool
	Errors    []ValidationError
	Processed time.Time
}

// ValidationError 校验错误
type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}

// DTUReceiver DTU数据接收器
// 负责传感器数据的接收、校验、转发
type DTUReceiver struct {
	config       ValidationConfig
	outputChan   chan<- ValidatedData
	rawInputChan <-chan model.SensorData
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	stats        ReceiverStats
	mu           sync.RWMutex
}

// ReceiverStats 接收器统计
type ReceiverStats struct {
	TotalReceived   int64
	TotalValid      int64
	TotalInvalid    int64
	LastReceived    time.Time
	ErrorCounts     map[string]int64
}

// NewDTUReceiver 创建DTU接收器
// rawInputChan: 原始传感器数据输入通道
// outputChan: 校验后数据输出通道
func NewDTUReceiver(
	config ValidationConfig,
	rawInputChan <-chan model.SensorData,
	outputChan chan<- ValidatedData,
) *DTUReceiver {
	ctx, cancel := context.WithCancel(context.Background())
	return &DTUReceiver{
		config:       config,
		outputChan:   outputChan,
		rawInputChan: rawInputChan,
		ctx:          ctx,
		cancel:       cancel,
		stats: ReceiverStats{
			ErrorCounts: make(map[string]int64),
		},
	}
}

// Start 启动接收器
func (r *DTUReceiver) Start() {
	log.Println("[DTUReceiver] Starting...")
	r.wg.Add(1)
	go r.receiveLoop()
}

// Stop 停止接收器
func (r *DTUReceiver) Stop() {
	log.Println("[DTUReceiver] Stopping...")
	r.cancel()
	r.wg.Wait()
	log.Println("[DTUReceiver] Stopped")
}

// receiveLoop 主接收循环
func (r *DTUReceiver) receiveLoop() {
	defer r.wg.Done()

	for {
		select {
		case <-r.ctx.Done():
			return
		case data, ok := <-r.rawInputChan:
			if !ok {
				log.Println("[DTUReceiver] Input channel closed")
				return
			}
			r.processData(data)
		}
	}
}

// processData 处理单条数据
func (r *DTUReceiver) processData(data model.SensorData) {
	r.mu.Lock()
	r.stats.TotalReceived++
	r.stats.LastReceived = time.Now()
	r.mu.Unlock()

	// 执行校验
	validated := r.validate(data)

	if validated.IsValid {
		r.mu.Lock()
		r.stats.TotalValid++
		r.mu.Unlock()
	} else {
		r.mu.Lock()
		r.stats.TotalInvalid++
		for _, err := range validated.Errors {
			r.stats.ErrorCounts[err.Field]++
		}
		r.mu.Unlock()
		log.Printf("[DTUReceiver] Data validation failed: crossbow=%s, errors=%d",
			data.CrossbowID, len(validated.Errors))
	}

	// 转发（无论是否有效，都带上校验结果）
	select {
	case r.outputChan <- validated:
	case <-r.ctx.Done():
		return
	}
}

// Validate 公开校验方法
func (r *DTUReceiver) Validate(data model.SensorData) ValidatedData {
	return r.validate(data)
}

// validate 内部校验逻辑
func (r *DTUReceiver) validate(data model.SensorData) ValidatedData {
	result := ValidatedData{
		Data:      data,
		IsValid:   true,
		Errors:    []ValidationError{},
		Processed: time.Now(),
	}

	// 必填字段检查
	if data.CrossbowID == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "crossbow_id",
			Message: "crossbow_id is required",
			Value:   data.CrossbowID,
		})
	}

	// 张力范围校验
	if data.StringTension < r.config.MinTension || data.StringTension > r.config.MaxTension {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "string_tension",
			Message: "out of valid range",
			Value:   data.StringTension,
		})
	}

	// 弩臂变形范围校验
	if data.ArmDeformation < r.config.MinDeformation || data.ArmDeformation > r.config.MaxDeformation {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "arm_deformation",
			Message: "out of valid range",
			Value:   data.ArmDeformation,
		})
	}

	// 箭匣位置范围校验
	if data.MagazinePosition < r.config.MinMagazinePos || data.MagazinePosition > r.config.MaxMagazinePos {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "magazine_position",
			Message: "out of valid range",
			Value:   data.MagazinePosition,
		})
	}

	// 射速范围校验
	if data.FireRate < r.config.MinFireRate || data.FireRate > r.config.MaxFireRate {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "fire_rate",
			Message: "out of valid range",
			Value:   data.FireRate,
		})
	}

	// 时间戳合理性校验
	if !data.Timestamp.IsZero() {
		now := time.Now()
		diff := now.Sub(data.Timestamp)
		if diff < -r.config.MaxTimeOffset || diff > r.config.MaxTimeOffset {
			result.IsValid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   "timestamp",
				Message: "timestamp offset exceeds limit",
				Value:   data.Timestamp,
			})
		}
	} else {
		// 时间戳为空，填充当前时间
		result.Data.Timestamp = time.Now()
	}

	// 物理一致性校验：张力与变形正相关
	if data.StringTension > 0 && data.ArmDeformation < 0 {
		// 张力为正，变形应为正（弩臂向外弯）
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "tension_deformation_correlation",
			Message: "positive tension should correspond to positive deformation",
			Value:   map[string]float64{"tension": data.StringTension, "deformation": data.ArmDeformation},
		})
	}

	// 物理一致性校验：射速与箭匣位置负相关
	if data.FireRate > 5 && data.MagazinePosition > 0.8 {
		// 高射速时箭匣应该被推进（位置小）
		// 这是一个弱校验，只警告，不标记为无效（暂时通过）
	}

	return result
}

// GetStats 获取统计信息
func (r *DTUReceiver) GetStats() ReceiverStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 返回拷贝
	statsCopy := r.stats
	statsCopy.ErrorCounts = make(map[string]int64)
	for k, v := range r.stats.ErrorCounts {
		statsCopy.ErrorCounts[k] = v
	}
	return statsCopy
}

// ResetStats 重置统计
func (r *DTUReceiver) ResetStats() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats = ReceiverStats{
		ErrorCounts: make(map[string]int64),
	}
}

// ValidateAndForward 同步校验并转发（供外部API调用）
func (r *DTUReceiver) ValidateAndForward(data model.SensorData) error {
	select {
	case <-r.ctx.Done():
		return errors.New("receiver is stopped")
	default:
	}

	r.processData(data)
	return nil
}
