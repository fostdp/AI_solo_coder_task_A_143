package simulation

import (
	"math"

	"gonum.org/v1/gonum/mat"
)

// CamMechanism 凸轮机构
// 采用等径凸轮（Conjugate Cam）设计，确保正反行程均有精确控制
type CamMechanism struct {
	Params          CamParams
	ProfilePoints   []CamProfilePoint
	CurrentAngle    float64       // 当前凸轮转角 φ [rad]
	CurrentFollower CamFollowerState
}

// NewCamMechanism 创建凸轮机构
func NewCamMechanism(params CamParams) *CamMechanism {
	return &CamMechanism{
		Params: params,
	}
}

// GenerateProfile 生成凸轮轮廓曲线
// 等径凸轮（也叫对心凸轮）的轮廓曲线满足：r(φ) + r(φ + π) = 常数
// 采用正弦加速度运动规律（简谐运动）实现平滑的从动件运动
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
// 等径凸轮轮廓计算:
// 基圆半径: Rb, 升程: h
// 推程角: Φ, 远休止角: Φs, 回程角: Φ', 近休止角: Φ's
// 本实现采用正弦加速度运动规律（无冲击）
func (cm *CamMechanism) CalculateProfilePoint(φ float64) CamProfilePoint {
	params := cm.Params
	Rb := params.BaseRadius
	h := params.Lift

	// 分段定义运动角
	Φ := math.Pi / 2.0      // 推程角 90°
	Φs := math.Pi / 4.0     // 远休止角 45°
	Φprime := math.Pi / 2.0 // 回程角 90°

	// 归一化角度到 [0, 2π)
	φNorm := math.Mod(φ, 2*math.Pi)
	if φNorm < 0 {
		φNorm += 2 * math.Pi
	}

	// 计算从动件位移 s(φ)
	s := 0.0
	ds_dφ := 0.0
	d2s_dφ2 := 0.0

	if φNorm < Φ {
		// 推程段: 正弦加速度运动规律
		// s(φ) = h * [ (φ/Φ) - (1/(2π))*sin(2πφ/Φ) ]
		// 该运动规律的特点: 速度连续, 加速度连续但有突变, 跃度有冲击
		// 属于"柔性冲击", 适用于中速场合
		ratio := φNorm / Φ
		s = h * (ratio - math.Sin(2*math.Pi*ratio)/(2*math.Pi))
		ds_dφ = h / Φ * (1 - math.Cos(2*math.Pi*ratio))
		d2s_dφ2 = 2 * math.Pi * h / (Φ * Φ) * math.Sin(2*math.Pi*ratio)
	} else if φNorm < Φ+Φs {
		// 远休止段
		s = h
		ds_dφ = 0.0
		d2s_dφ2 = 0.0
	} else if φNorm < Φ+Φs+Φprime {
		// 回程段: 正弦加速度运动规律
		ratio := (φNorm - Φ - Φs) / Φprime
		s = h * (1 - ratio + math.Sin(2*math.Pi*ratio)/(2*math.Pi))
		ds_dφ = -h / Φprime * (1 - math.Cos(2*math.Pi*ratio))
		d2s_dφ2 = -2 * math.Pi * h / (Φprime * Φprime) * math.Sin(2*math.Pi*ratio)
	} else {
		// 近休止段
		s = 0.0
		ds_dφ = 0.0
		d2s_dφ2 = 0.0
	}

	// 向径 r(φ) = Rb + s(φ)
	r := Rb + s

	// 直角坐标 (极坐标转换)
	// x = r(φ) * cos(φ), y = r(φ) * sin(φ)
	x := r * math.Cos(φ)
	y := r * math.Sin(φ)

	// 计算切线方向和法向量
	// 切线向量: dr/dφ = (ds/dφ*cosφ - r*sinφ, ds/dφ*sinφ + r*cosφ)
	dx_dφ := ds_dφ*math.Cos(φ) - r*math.Sin(φ)
	dy_dφ := ds_dφ*math.Sin(φ) + r*math.Cos(φ)

	// 切线方向单位向量
	tangentMag := math.Sqrt(dx_dφ*dx_dφ + dy_dφ*dy_dφ)
	tx := dx_dφ / tangentMag
	ty := dy_dφ / tangentMag

	// 法向量（指向凸轮中心的内侧法线）
	// 法线方向为切线旋转90°: n = (-ty, tx)
	nx := -ty
	ny := tx

	// 计算曲率 κ(φ)
	// 曲率公式: κ = |x'y'' - x''y'| / (x'² + y'²)^(3/2)
	d2x_dφ2 := (d2s_dφ2 - r)*math.Cos(φ) - 2*ds_dφ*math.Sin(φ)
	d2y_dφ2 := (d2s_dφ2 - r)*math.Sin(φ) + 2*ds_dφ*math.Cos(φ)

	numerator := math.Abs(dx_dφ*d2y_dφ2 - d2x_dφ2*dy_dφ)
	denominator := math.Pow(dx_dφ*dx_dφ + dy_dφ*dy_dφ, 1.5)
	curvature := numerator / denominator

	// 压力角计算
	// tan(α) = |ds/dφ| / (Rb + s)
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

// CalculateFollowerMotion 计算从动件运动学参数
// 输入: 凸轮转角 φ [rad], 角速度 ω [rad/s]
// 输出: 从动件位移 s, 速度 v, 加速度 a, 跃度 j
//
// 正弦加速度运动规律（推程）:
//   s(φ) = h[ φ/Φ - (1/2π)sin(2πφ/Φ) ]
//   v(φ) = ds/dt = (ds/dφ)(dφ/dt) = ω h/Φ [1 - cos(2πφ/Φ)]
//   a(φ) = dv/dt = ω² 2πh/Φ² sin(2πφ/Φ)
//   j(φ) = da/dt = ω³ 4π²h/Φ³ cos(2πφ/Φ)
func (cm *CamMechanism) CalculateFollowerMotion(φ float64, ω float64) CamFollowerState {
	params := cm.Params
	h := params.Lift

	Φ := math.Pi / 2.0      // 推程角
	Φs := math.Pi / 4.0     // 远休止角
	Φprime := math.Pi / 2.0 // 回程角

	φNorm := math.Mod(φ, 2*math.Pi)
	if φNorm < 0 {
		φNorm += 2 * math.Pi
	}

	var s, ds_dφ, d2s_dφ2, d3s_dφ3 float64

	if φNorm < Φ {
		// 推程段: 正弦加速度
		ratio := φNorm / Φ
		arg := 2 * math.Pi * ratio

		// 位移方程: s = h[ φ/Φ - (1/2π)sin(2πφ/Φ) ]
		s = h * (ratio - math.Sin(arg)/(2*math.Pi))

		// 一阶导数: ds/dφ = h/Φ [1 - cos(2πφ/Φ)]
		ds_dφ = h / Φ * (1 - math.Cos(arg))

		// 二阶导数: d²s/dφ² = 2πh/Φ² sin(2πφ/Φ)
		d2s_dφ2 = 2 * math.Pi * h / (Φ * Φ) * math.Sin(arg)

		// 三阶导数: d³s/dφ³ = 4π²h/Φ³ cos(2πφ/Φ)
		d3s_dφ3 = 4 * math.Pi * math.Pi * h / (Φ * Φ * Φ) * math.Cos(arg)

	} else if φNorm < Φ+Φs {
		// 远休止段: s = h, 各阶导数为0
		s = h
		ds_dφ = 0.0
		d2s_dφ2 = 0.0
		d3s_dφ3 = 0.0

	} else if φNorm < Φ+Φs+Φprime {
		// 回程段: 正弦加速度
		ratio := (φNorm - Φ - Φs) / Φprime
		arg := 2 * math.Pi * ratio

		// 位移方程: s = h[ 1 - φ/Φ' + (1/2π)sin(2πφ/Φ') ]
		s = h * (1 - ratio + math.Sin(arg)/(2*math.Pi))

		// 一阶导数: ds/dφ = -h/Φ' [1 - cos(2πφ/Φ')]
		ds_dφ = -h / Φprime * (1 - math.Cos(arg))

		// 二阶导数: d²s/dφ² = -2πh/Φ'^2 sin(2πφ/Φ')
		d2s_dφ2 = -2 * math.Pi * h / (Φprime * Φprime) * math.Sin(arg)

		// 三阶导数: d³s/dφ³ = -4π²h/Φ'^3 cos(2πφ/Φ')
		d3s_dφ3 = -4 * math.Pi * math.Pi * h / (Φprime * Φprime * Φprime) * math.Cos(arg)

	} else {
		// 近休止段
		s = 0.0
		ds_dφ = 0.0
		d2s_dφ2 = 0.0
		d3s_dφ3 = 0.0
	}

	// 转换为时间导数
	// 速度: v = ds/dt = (ds/dφ)(dφ/dt) = ω * ds/dφ
	velocity := ω * ds_dφ

	// 加速度: a = dv/dt = d/dt(ω ds/dφ) = ω² d²s/dφ² (假设ω恒定)
	acceleration := ω * ω * d2s_dφ2

	// 跃度: j = da/dt = ω³ d³s/dφ³
	jerk := ω * ω * ω * d3s_dφ3

	// 压力角计算
	// tan(α) = |ds/dφ| / (Rb + s)
	pressureAngle := math.Atan(math.Abs(ds_dφ) / (params.BaseRadius + s))

	return CamFollowerState{
		Displacement:  s,
		Velocity:      velocity,
		Acceleration:  acceleration,
		Jerk:          jerk,
		PressureAngle: pressureAngle,
	}
}

// CalculateContactForce 计算凸轮-从动件接触力
// 输入: 从动件运动状态, 从动件等效质量 meq, 外载荷 Fload
// 输出: 法向接触力 Fn
//
// 力平衡方程（沿从动件运动方向）:
//   Fn * cos(α) - Ff * sin(α) = meq * a + F_load + F_spring + F_damper
//   其中:
//     Ff = μ * Fn (库仑摩擦力)
//     F_spring = k * s (弹簧力)
//     F_damper = c * v (阻尼力)
//
// 整理得:
//   Fn = (meq * a + F_load + k*s + c*v) / (cosα - μ*sinα)
func (cm *CamMechanism) CalculateContactForce(
	follower CamFollowerState,
	meq float64,
	springK float64,
	damperC float64,
	Fload float64,
	mu float64,
) float64 {
	α := follower.PressureAngle
	s := follower.Displacement
	v := follower.Velocity
	a := follower.Acceleration

	// 弹簧力
	Fspring := springK * s

	// 阻尼力
	Fdamper := damperC * v

	// 惯性力
	Finertial := meq * a

	// 分母（力的几何投影系数）
	denominator := math.Cos(α) - mu*math.Sin(α)
	if math.Abs(denominator) < 1e-10 {
		denominator = 1e-10 * math.Copysign(1.0, denominator)
	}

	// 法向接触力
	Fn := (Finertial + Fload + Fspring + Fdamper) / denominator

	return math.Max(0.0, Fn) // 不能有拉力，只能有压力
}

// AutoLoadingController 自动装填时序控制器
// 控制弩的自动装填过程，包括：拉弦、锁止、装箭、释放
type AutoLoadingController struct {
	CurrentPhase  int
	PhaseStartTime float64
	Sequence      []LoadingSequence
	CamMechanism  *CamMechanism
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
// 输入: 当前时间 t [s]
// 输出: 当前阶段索引, 是否完成全部装填
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
// 输出:
//   camTargetAngle: 凸轮目标转角 [rad]
//   pawlCommand: 棘爪指令 (-1:脱开, 0:保持, 1:啮合)
//   arrowLoad: 是否装箭
//   trigger: 是否触发发射
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
		// 初始位置: 凸轮在近休止位置
		camTargetAngle = 0.0
		pawlCommand = 0
		arrowLoad = false
		trigger = false

	case 1:
		// 拉弦阶段: 凸轮旋转拉弦
		// 推程角 90°，使用正弦加速曲线
		camTargetAngle = progress * math.Pi / 2.0
		pawlCommand = 0
		arrowLoad = false
		trigger = false

	case 2:
		// 锁止阶段: 棘爪啮合锁止
		camTargetAngle = math.Pi/2.0 + math.Pi/4.0*progress // 进入远休止
		pawlCommand = 1
		arrowLoad = false
		trigger = false

	case 3:
		// 装箭阶段
		camTargetAngle = math.Pi / 2.0
		pawlCommand = 1
		arrowLoad = true
		trigger = false

	case 4:
		// 触发阶段: 棘爪脱开
		camTargetAngle = math.Pi/2.0 + math.Pi/4.0 // 远休止位置
		pawlCommand = -1
		arrowLoad = true
		trigger = false

	case 5:
		// 发射阶段: 凸轮继续旋转完成回程
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
// 一般推荐: α_max ≤ 30°~45° (直动从动件)
//          α_max ≤ 40°~50° (摆动从动件)
func (cm *CamMechanism) CheckPressureAngle(α float64) bool {
	maxAllowed := cm.Params.PressureAngle
	if maxAllowed <= 0 {
		maxAllowed = 30.0 * math.Pi / 180.0 // 默认30°
	}
	return α <= maxAllowed
}

// CalculateTorque 计算驱动凸轮所需扭矩
// 输入: 接触力 Fn, 压力角 α, 基圆半径 Rb, 从动件位移 s
// 扭矩: T = Fn * (Rb + s) * sin(α)
// 物理意义: 扭矩等于法向力乘以力臂，力臂是接触点到凸轮中心的垂直距离
func (cm *CamMechanism) CalculateTorque(Fn float64, follower CamFollowerState) float64 {
	α := follower.PressureAngle
	s := follower.Displacement
	Rb := cm.Params.BaseRadius

	// T = Fn * (Rb + s) * sin(α)
	return Fn * (Rb + s) * math.Sin(α)
}
