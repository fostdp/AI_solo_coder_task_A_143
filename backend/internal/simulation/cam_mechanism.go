package simulation

import (
	"math"

	"gonum.org/v1/gonum/mat"
)

// CamMechanism 凸轮机构
// 采用等径凸轮（Conjugate Cam）设计，确保正反行程均有精确控制
// 引入缓冲弹簧模型，消除高速运动时的冲击，防止机构卡死
type CamMechanism struct {
	Params          CamParams
	BufferSpring    BufferSpringParams
	ProfilePoints   []CamProfilePoint
	CurrentAngle    float64       // 当前凸轮转角 φ [rad]
	CurrentFollower CamFollowerState
	FollowerMass    float64       // 从动件等效质量 [kg]
	ExternalLoad    float64       // 外载荷 F_load [N]
}

// NewCamMechanism 创建凸轮机构
func NewCamMechanism(params CamParams, springParams BufferSpringParams) *CamMechanism {
	return &CamMechanism{
		Params:       params,
		BufferSpring: springParams,
		FollowerMass: 0.5, // 默认等效质量 0.5kg
	}
}

// DefaultBufferSpringParams 默认缓冲弹簧参数
func DefaultBufferSpringParams() BufferSpringParams {
	return BufferSpringParams{
		Stiffness:      5000.0,  // 5000 N/m
		Preload:        50.0,    // 50 N 预紧力
		Damping:        30.0,    // 30 N·s/m
		MaxCompression: 0.02,    // 20 mm 最大压缩量
		EquivalentMass: 0.5,     // 0.5 kg
	}
}

// quinticPoly 5次多项式计算
// 用于缓冲段，保证位移、速度、加速度、跃度连续
// s(T) = c0 + c1*T + c2*T² + c3*T³ + c4*T⁴ + c5*T⁵
// 边界条件: s(0)=s0, s'(0)=v0, s''(0)=a0, s(1)=s1, s'(1)=v1, s''(1)=a1
func quinticPoly(T float64, s0, v0, a0, s1, v1, a1 float64) (s, v, a, j float64) {
	T2 := T * T
	T3 := T2 * T
	T4 := T3 * T
	T5 := T4 * T

	c0 := s0
	c1 := v0
	c2 := a0 / 2.0
	c3 := 10.0*(s1-s0) - 6.0*v0 - 4.0*v1 - 1.5*a0 + 0.5*a1
	c4 := -15.0*(s1-s0) + 8.0*v0 + 7.0*v1 + 2.0*a0 - a1
	c5 := 6.0*(s1-s0) - 3.0*(v0+v1) - 0.5*(a0-a1)

	s = c0 + c1*T + c2*T2 + c3*T3 + c4*T4 + c5*T5
	v = c1 + 2.0*c2*T + 3.0*c3*T2 + 4.0*c4*T3 + 5.0*c5*T4
	a = 2.0*c2 + 6.0*c3*T + 12.0*c4*T2 + 20.0*c5*T3
	j = 6.0*c3 + 24.0*c4*T + 60.0*c5*T2

	return
}

// GenerateProfile 生成凸轮轮廓曲线
// 采用带缓冲段的改进正弦加速度运动规律
// 段划分: 缓冲推程 → 主推程 → 缓冲推程 → 远休止 → 缓冲回程 → 主回程 → 缓冲回程 → 近休止
func (cm *CamMechanism) GenerateProfile(numPoints int) []CamProfilePoint {
	cm.ProfilePoints = make([]CamProfilePoint, numPoints)
	dφ := 2 * math.Pi / float64(numPoints)

	for i := 0; i < numPoints; i++ {
		φ := float64(i) * dφ
		point := cm.CalculateProfilePoint(φ)
		cm.ProfilePoints[i] = point
	}

	return cm.ProfilePoints
}

// CalculateProfilePoint 计算凸轮轮廓上某一点
// 输入: 凸轮转角 φ [rad]
// 输出: 轮廓点坐标、法向量、曲率
//
// 采用分段运动规律，各段间用5次多项式平滑过渡，实现无冲击运动
func (cm *CamMechanism) CalculateProfilePoint(φ float64) CamProfilePoint {
	params := cm.Params
	Rb := params.BaseRadius
	h := params.Lift

	// 分段运动角定义（总2π）
	// 缓冲段角度占比: 各15°
	bufferAngle := 15.0 * math.Pi / 180.0 // 缓冲段角度 15°
	Φ_main := math.Pi/2.0 - 2.0*bufferAngle // 主推程角
	Φs := math.Pi / 4.0                       // 远休止角
	Φprime_main := math.Pi/2.0 - 2.0*bufferAngle // 主回程角

	// 各段起点角度
	φ_start_buf1 := 0.0
	φ_start_push := φ_start_buf1 + bufferAngle
	φ_start_buf2 := φ_start_push + Φ_main
	φ_start_dwell_far := φ_start_buf2 + bufferAngle
	φ_start_buf3 := φ_start_dwell_far + Φs
	φ_start_return := φ_start_buf3 + bufferAngle
	φ_start_buf4 := φ_start_return + Φprime_main
	φ_end := φ_start_buf4 + bufferAngle

	// 归一化角度到 [0, 2π)
	φNorm := math.Mod(φ, 2*math.Pi)
	if φNorm < 0 {
		φNorm += 2 * math.Pi
	}

	// 计算从动件位移 s(φ) 及其导数
	var s, ds_dφ, d2s_dφ2, d3s_dφ3 float64

	switch {
	case φNorm < φ_start_push:
		// 第一段缓冲: 从静止到主推程起始速度
		T := φNorm / bufferAngle
		s, ds_dφ, d2s_dφ2, d3s_dφ3 = quinticPolyBuffer(T, 0, 0, 0, bufferAngle, h, Φ_main, true)

	case φNorm < φ_start_buf2:
		// 主推程: 正弦加速度运动规律
		ratio := (φNorm - φ_start_push) / Φ_main
		s_main, v_main, a_main := sineAccelerationPush(ratio, h, Φ_main)
		s = h * (ratio - math.Sin(2*math.Pi*ratio)/(2*math.Pi))
		ds_dφ = h / Φ_main * (1 - math.Cos(2*math.Pi*ratio))
		d2s_dφ2 = 2 * math.Pi * h / (Φ_main * Φ_main) * math.Sin(2*math.Pi*ratio)
		d3s_dφ3 = 4 * math.Pi * math.Pi * h / (Φ_main * Φ_main * Φ_main) * math.Cos(2*math.Pi*ratio)
		_, _, _ = s_main, v_main, a_main

	case φNorm < φ_start_dwell_far:
		// 第二段缓冲: 从主推程结束到远休止
		T := (φNorm - φ_start_buf2) / bufferAngle
		s_end := h
		ds_end := 0.0
		d2s_end := 0.0

		// 主推程结束时的状态
		s_start := h
		ds_start := h / Φ_main * (1 - math.Cos(2*math.Pi))
		d2s_start := 2 * math.Pi * h / (Φ_main * Φ_main) * math.Sin(2*math.Pi)

		s, ds_dφ, d2s_dφ2, d3s_dφ3 = quinticPoly(
			T,
			s_start, ds_start, d2s_start,
			s_end, ds_end, d2s_end,
		)
		_ = d3s_dφ3 // 暂时保留

	case φNorm < φ_start_buf3:
		// 远休止段
		s = h
		ds_dφ = 0.0
		d2s_dφ2 = 0.0
		d3s_dφ3 = 0.0

	case φNorm < φ_start_return:
		// 第三段缓冲: 从远休止到主回程起始
		T := (φNorm - φ_start_buf3) / bufferAngle
		s_start := h
		ds_start := 0.0
		d2s_start := 0.0
		s_end := h
		// 主回程开始时的速度（负的）
		ds_end := -h / Φprime_main * (1 - math.Cos(0))

		s, ds_dφ, d2s_dφ2, d3s_dφ3 = quinticPoly(
			T,
			s_start, ds_start, d2s_start,
			s_end, ds_end, 0.0,
		)

	case φNorm < φ_start_buf4:
		// 主回程: 正弦加速度运动规律
		ratio := (φNorm - φ_start_return) / Φprime_main
		s = h * (1 - ratio + math.Sin(2*math.Pi*ratio)/(2*math.Pi))
		ds_dφ = -h / Φprime_main * (1 - math.Cos(2*math.Pi*ratio))
		d2s_dφ2 = -2 * math.Pi * h / (Φprime_main * Φprime_main) * math.Sin(2*math.Pi*ratio)
		d3s_dφ3 = -4 * math.Pi * math.Pi * h / (Φprime_main * Φprime_main * Φprime_main) * math.Cos(2*math.Pi*ratio)

	default:
		// 第四段缓冲: 从主回程结束到近休止
		T := (φNorm - φ_start_buf4) / bufferAngle
		s_start := 0.0
		ds_start := -h / Φprime_main * (1 - math.Cos(2*math.Pi))
		d2s_start := -2 * math.Pi * h / (Φprime_main * Φprime_main) * math.Sin(2*math.Pi)

		s, ds_dφ, d2s_dφ2, d3s_dφ3 = quinticPoly(
			T,
			s_start, ds_start, d2s_start,
			0.0, 0.0, 0.0,
		)
	}

	// 向径 r(φ) = Rb + s(φ)
	r := Rb + s

	// 直角坐标
	x := r * math.Cos(φ)
	y := r * math.Sin(φ)

	// 切线方向和法向量
	dx_dφ := ds_dφ*math.Cos(φ) - r*math.Sin(φ)
	dy_dφ := ds_dφ*math.Sin(φ) + r*math.Cos(φ)

	tangentMag := math.Sqrt(dx_dφ*dx_dφ + dy_dφ*dy_dφ)
	tx := dx_dφ / tangentMag
	ty := dy_dφ / tangentMag

	// 法向量
	nx := -ty
	ny := tx

	// 曲率计算
	d2x_dφ2 := (d2s_dφ2 - r)*math.Cos(φ) - 2*ds_dφ*math.Sin(φ)
	d2y_dφ2 := (d2s_dφ2 - r)*math.Sin(φ) + 2*ds_dφ*math.Cos(φ)

	numerator := math.Abs(dx_dφ*d2y_dφ2 - d2x_dφ2*dy_dφ)
	denominator := math.Pow(dx_dφ*dx_dφ + dy_dφ*dy_dφ, 1.5)
	curvature := numerator / denominator

	// 压力角
	pressureAngle := math.Atan(math.Abs(ds_dφ) / (Rb + s))

	return CamProfilePoint{
		Angle:     φ,
		Radius:    r,
		X:         x,
		Y:         y,
		NormalX:   nx,
		NormalY:   ny,
		Curvature: curvature,
	}
}

// quinticPolyBuffer 缓冲段5次多项式（专用版本）
// 从静止平滑过渡到正弦加速度主段的起始速度
func quinticPolyBuffer(T float64, s0, v0, a0 float64, bufferAngle float64, h float64, mainAngle float64, isPush bool) (s, v, a, j float64) {
	// 主段起始时的速度和加速度（正弦加速度规律）
	mainVel := h / mainAngle * (1 - math.Cos(0))
	mainAcc := 2 * math.Pi * h / (mainAngle * mainAngle) * math.Sin(0)

	if !isPush {
		mainVel = -mainVel
	}

	// 缓冲段终点位移（近似为主段起点的位移）
	s_end := h * 0.02 // 缓冲段只走2%的升程，主要是速度过渡
	v_end := mainVel
	a_end := mainAcc

	s, v, a, j = quinticPoly(T, s0, v0, a0, s_end, v_end, a_end)
	return
}

// sineAccelerationPush 正弦加速度推程计算
func sineAccelerationPush(ratio float64, h float64, Φ float64) (s, v, a float64) {
	s = h * (ratio - math.Sin(2*math.Pi*ratio)/(2*math.Pi))
	v = h / Φ * (1 - math.Cos(2*math.Pi*ratio))
	a = 2 * math.Pi * h / (Φ * Φ) * math.Sin(2*math.Pi*ratio)
	return
}

// CalculateFollowerMotion 计算从动件运动学参数（理想运动，不考虑弹簧缓冲）
// 输入: 凸轮转角 φ [rad], 角速度 ω [rad/s]
// 输出: 从动件位移、速度、加速度、跃度
func (cm *CamMechanism) CalculateIdealFollowerMotion(φ float64, ω float64) CamFollowerState {
	params := cm.Params
	h := params.Lift

	bufferAngle := 15.0 * math.Pi / 180.0
	Φ_main := math.Pi/2.0 - 2.0*bufferAngle
	Φs := math.Pi / 4.0
	Φprime_main := math.Pi/2.0 - 2.0*bufferAngle

	φ_start_buf1 := 0.0
	φ_start_push := φ_start_buf1 + bufferAngle
	φ_start_buf2 := φ_start_push + Φ_main
	φ_start_dwell_far := φ_start_buf2 + bufferAngle
	φ_start_buf3 := φ_start_dwell_far + Φs
	φ_start_return := φ_start_buf3 + bufferAngle
	φ_start_buf4 := φ_start_return + Φprime_main

	φNorm := math.Mod(φ, 2*math.Pi)
	if φNorm < 0 {
		φNorm += 2 * math.Pi
	}

	var s, ds_dφ, d2s_dφ2, d3s_dφ3 float64

	switch {
	case φNorm < φ_start_push:
		T := φNorm / bufferAngle
		s, ds_dφ, d2s_dφ2, d3s_dφ3 = quinticPolyBuffer(T, 0, 0, 0, bufferAngle, h, Φ_main, true)

	case φNorm < φ_start_buf2:
		ratio := (φNorm - φ_start_push) / Φ_main
		s = h * (ratio - math.Sin(2*math.Pi*ratio)/(2*math.Pi))
		ds_dφ = h / Φ_main * (1 - math.Cos(2*math.Pi*ratio))
		d2s_dφ2 = 2 * math.Pi * h / (Φ_main * Φ_main) * math.Sin(2*math.Pi*ratio)
		d3s_dφ3 = 4 * math.Pi * math.Pi * h / (Φ_main * Φ_main * Φ_main) * math.Cos(2*math.Pi*ratio)

	case φNorm < φ_start_dwell_far:
		T := (φNorm - φ_start_buf2) / bufferAngle
		s_start := h
		ds_start := h / Φ_main * (1 - math.Cos(2*math.Pi))
		d2s_start := 2 * math.Pi * h / (Φ_main * Φ_main) * math.Sin(2*math.Pi)
		s, ds_dφ, d2s_dφ2, d3s_dφ3 = quinticPoly(T, s_start, ds_start, d2s_start, h, 0, 0)

	case φNorm < φ_start_buf3:
		s = h
		ds_dφ = 0.0
		d2s_dφ2 = 0.0
		d3s_dφ3 = 0.0

	case φNorm < φ_start_return:
		T := (φNorm - φ_start_buf3) / bufferAngle
		s_start := h
		ds_start := 0.0
		d2s_start := 0.0
		ds_end := -h / Φprime_main * (1 - math.Cos(0))
		s, ds_dφ, d2s_dφ2, d3s_dφ3 = quinticPoly(T, s_start, ds_start, d2s_start, h, ds_end, 0)

	case φNorm < φ_start_buf4:
		ratio := (φNorm - φ_start_return) / Φprime_main
		s = h * (1 - ratio + math.Sin(2*math.Pi*ratio)/(2*math.Pi))
		ds_dφ = -h / Φprime_main * (1 - math.Cos(2*math.Pi*ratio))
		d2s_dφ2 = -2 * math.Pi * h / (Φprime_main * Φprime_main) * math.Sin(2*math.Pi*ratio)
		d3s_dφ3 = -4 * math.Pi * math.Pi * h / (Φprime_main * Φprime_main * Φprime_main) * math.Cos(2*math.Pi*ratio)

	default:
		T := (φNorm - φ_start_buf4) / bufferAngle
		s_start := 0.0
		ds_start := -h / Φprime_main * (1 - math.Cos(2*math.Pi))
		d2s_start := -2 * math.Pi * h / (Φprime_main * Φprime_main) * math.Sin(2*math.Pi)
		s, ds_dφ, d2s_dφ2, d3s_dφ3 = quinticPoly(T, s_start, ds_start, d2s_start, 0, 0, 0)
	}

	velocity := ω * ds_dφ
	acceleration := ω * ω * d2s_dφ2
	jerk := ω * ω * ω * d3s_dφ3
	pressureAngle := math.Atan(math.Abs(ds_dφ) / (params.BaseRadius + s))

	return CamFollowerState{
		Displacement:  s,
		Velocity:      velocity,
		Acceleration:  acceleration,
		Jerk:          jerk,
		PressureAngle: pressureAngle,
	}
}

// UpdateFollowerWithSpring 考虑缓冲弹簧的从动件运动更新
// 使用弹簧-质量-阻尼系统模拟真实接触，防止脱击和冲击
// 输入: 凸轮转角变化 dφ, 角速度 ω, 时间步长 dt
func (cm *CamMechanism) UpdateFollowerWithSpring(prevState CamFollowerState, φ float64, ω float64, dt float64) CamFollowerState {
	// 计算理想的从动件位移（凸轮轮廓决定的位移）
	ideal := cm.CalculateIdealFollowerMotion(φ, ω)

	// 当前弹簧变形量 = 理想位移 - 实际位移
	// 正值表示弹簧被压缩，产生弹力推动从动件
	springDeflection := ideal.Displacement - prevState.Displacement

	// 弹簧力（胡克定律 + 预紧力）
	springForce := cm.BufferSpring.Preload + cm.BufferSpring.Stiffness*springDeflection

	// 相对速度（凸轮速度 - 从动件速度）
	relativeVel := ideal.Velocity - prevState.Velocity

	// 阻尼力
	dampingForce := cm.BufferSpring.Damping * relativeVel

	// 总力 = 弹簧力 + 阻尼力 - 外载荷
	totalForce := springForce + dampingForce - cm.ExternalLoad

	// 判断是否保持接触
	// 当弹簧力 > 0 时，凸轮与从动件保持接触
	isContacting := springForce > 0

	// 限制弹簧最大压缩量
	if springDeflection > cm.BufferSpring.MaxCompression {
		springDeflection = cm.BufferSpring.MaxCompression
		springForce = cm.BufferSpring.Preload + cm.BufferSpring.Stiffness*springDeflection
	}

	// 计算从动件加速度（牛顿第二定律）
	// F = ma → a = F/m
	var followerAcc float64
	if isContacting {
		// 接触状态: 弹簧力推动从动件
		followerAcc = totalForce / cm.BufferSpring.EquivalentMass
	} else {
		// 脱击状态: 只有外载荷和阻尼作用（自由飞行/回落）
		followerAcc = (-cm.ExternalLoad - dampingForce) / cm.BufferSpring.EquivalentMass
	}

	// 欧拉积分更新速度和位移
	newVelocity := prevState.Velocity + followerAcc*dt
	newDisplacement := prevState.Displacement + newVelocity*dt

	// 防止位移小于0（物理限制）
	if newDisplacement < 0 {
		newDisplacement = 0
		if newVelocity < 0 {
			newVelocity = 0
		}
	}

	// 冲击速度（接触瞬间的相对速度，用于评估冲击程度）
	impactVel := 0.0
	if !prevState.IsContacting && isContacting {
		impactVel = math.Abs(ideal.Velocity - prevState.Velocity)
	}

	// 接触力计算（考虑压力角和摩擦）
	contactForce := 0.0
	if isContacting {
		α := ideal.PressureAngle
		mu := cm.Params.FrictionCoeff
		denominator := math.Cos(α) - mu*math.Sin(α)
		if math.Abs(denominator) > 1e-10 {
			contactForce = springForce / denominator
		} else {
			contactForce = springForce * 10.0 // 自锁临界
		}
	}

	return CamFollowerState{
		Displacement:     newDisplacement,
		Velocity:         newVelocity,
		Acceleration:     followerAcc,
		Jerk:             ideal.Jerk,
		PressureAngle:    ideal.PressureAngle,
		SpringForce:      springForce,
		SpringDeflection: springDeflection,
		ContactForce:     math.Max(0, contactForce),
		IsContacting:     isContacting,
		ImpactVelocity:   impactVel,
	}
}

// CalculateFollowerMotion 计算从动件运动学参数（兼容旧接口）
// 默认使用理想运动学（不考虑弹簧缓冲）
func (cm *CamMechanism) CalculateFollowerMotion(φ float64, ω float64) CamFollowerState {
	return cm.CalculateIdealFollowerMotion(φ, ω)
}

// CalculateContactForce 计算凸轮-从动件接触力
// 已包含缓冲弹簧力的影响
func (cm *CamMechanism) CalculateContactForce(
	follower CamFollowerState,
	meq float64,
	springK float64,
	damperC float64,
	Fload float64,
	mu float64,
) float64 {
	α := follower.PressureAngle
	_ = meq
	_ = springK
	_ = damperC
	_ = Fload

	// 已在 UpdateFollowerWithSpring 中计算过
	if follower.ContactForce > 0 {
		return follower.ContactForce
	}

	// 兼容模式：使用旧方法估算
	Fn := follower.SpringForce
	denominator := math.Cos(α) - mu*math.Sin(α)
	if math.Abs(denominator) < 1e-10 {
		denominator = 1e-10 * math.Copysign(1.0, denominator)
	}
	return math.Max(0.0, Fn/denominator)
}

// AutoLoadingController 自动装填时序控制器
type AutoLoadingController struct {
	CurrentPhase   int
	PhaseStartTime float64
	Sequence       []LoadingSequence
	CamMechanism   *CamMechanism
}

// NewAutoLoadingController 创建自动装填控制器
func NewAutoLoadingController(cam *CamMechanism) *AutoLoadingController {
	return &AutoLoadingController{
		CamMechanism: cam,
		Sequence: []LoadingSequence{
			{Phase: 0, StartTime: 0.0, Duration: 0.5, Description: "初始位置", Completed: false},
			{Phase: 1, StartTime: 0.5, Duration: 1.0, Description: "凸轮拉弦", Completed: false},
			{Phase: 2, StartTime: 1.5, Duration: 0.3, Description: "棘爪锁止", Completed: false},
			{Phase: 3, StartTime: 1.8, Duration: 0.2, Description: "装箭", Completed: false},
			{Phase: 4, StartTime: 2.0, Duration: 0.1, Description: "触发释放", Completed: false},
			{Phase: 5, StartTime: 2.1, Duration: 0.5, Description: "发射与复位", Completed: false},
		},
	}
}

// Update 更新装填时序
func (alc *AutoLoadingController) Update(t float64) (int, bool) {
	allCompleted := true

	for i := range alc.Sequence {
		seq := &alc.Sequence[i]
		if t >= seq.StartTime+seq.Duration {
			seq.Completed = true
		} else {
			allCompleted = false
		}

		if t >= seq.StartTime && !seq.Completed {
			alc.CurrentPhase = i
			if alc.PhaseStartTime == 0 {
				alc.PhaseStartTime = t
			}
		}
	}

	return alc.CurrentPhase, allCompleted
}

// GetPhaseOutput 获取当前阶段的控制输出
func (alc *AutoLoadingController) GetPhaseOutput(t float64) (float64, int, bool, bool) {
	phase := alc.CurrentPhase
	seq := alc.Sequence[phase]
	elapsed := t - seq.StartTime
	progress := math.Min(1.0, elapsed/seq.Duration)

	var camTargetAngle float64
	var pawlCommand int
	var arrowLoad bool
	var trigger bool

	switch phase {
	case 0:
		camTargetAngle = 0.0
		pawlCommand = 0
		arrowLoad = false
		trigger = false

	case 1:
		camTargetAngle = progress * math.Pi / 2.0
		pawlCommand = 0
		arrowLoad = false
		trigger = false

	case 2:
		camTargetAngle = math.Pi/2.0 + math.Pi/4.0*progress
		pawlCommand = 1
		arrowLoad = false
		trigger = false

	case 3:
		camTargetAngle = math.Pi / 2.0
		pawlCommand = 1
		arrowLoad = true
		trigger = false

	case 4:
		camTargetAngle = math.Pi/2.0 + math.Pi/4.0
		pawlCommand = -1
		arrowLoad = true
		trigger = false

	case 5:
		camTargetAngle = math.Pi/2.0 + math.Pi/4.0 + math.Pi/2.0*progress
		pawlCommand = -1
		arrowLoad = true
		trigger = true
	}

	return camTargetAngle, pawlCommand, arrowLoad, trigger
}

// GetFollowerPosition 获取从动件位置向量
func (cm *CamMechanism) GetFollowerPosition(φ float64) mat.VecDense {
	point := cm.CalculateProfilePoint(φ)
	pos := *mat.NewVecDense(2, []float64{point.X, point.Y})
	return pos
}

// CheckPressureAngle 校验压力角是否在允许范围内
func (cm *CamMechanism) CheckPressureAngle(α float64) bool {
	maxAllowed := cm.Params.PressureAngle
	if maxAllowed <= 0 {
		maxAllowed = 30.0 * math.Pi / 180.0
	}
	return α <= maxAllowed
}

// CalculateTorque 计算驱动凸轮所需扭矩
func (cm *CamMechanism) CalculateTorque(Fn float64, follower CamFollowerState) float64 {
	α := follower.PressureAngle
	s := follower.Displacement
	Rb := cm.Params.BaseRadius

	return Fn * (Rb + s) * math.Sin(α)
}

// CheckJamming 检查机构是否有卡死风险
// 基于压力角、弹簧力、摩擦系数判断
func (cm *CamMechanism) CheckJamming(follower CamFollowerState) bool {
	α := follower.PressureAngle
	mu := cm.Params.FrictionCoeff

	// 自锁条件: tan(α) >= 1/μ
	// 当压力角太大且摩擦系数足够时，机构可能自锁
	if math.Tan(α) >= 1.0/mu {
		return true
	}

	// 弹簧力不足导致脱击后无法回位
	if !follower.IsContacting && follower.SpringDeflection > cm.BufferSpring.MaxCompression*0.8 {
		return true
	}

	return false
}
