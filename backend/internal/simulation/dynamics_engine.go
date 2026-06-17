package simulation

import (
	"math"

	"gonum.org/v1/gonum/mat"
)

// DynamicsEngine 多刚体动力学引擎
// 基于欧拉-拉格朗日方程建立系统动力学模型
// 使用四阶龙格-库塔法（RK4）进行数值积分
type DynamicsEngine struct {
	BowArmLeft     BowArmParams
	BowArmRight    BowArmParams
	BowString      BowStringParams
	Cam            CamParams
	PawlRatchet    PawlRatchetParams
	Arrow          ArrowParams
	Config         SimulationConfig

	CamMechanism      *CamMechanism
	LoadingController *AutoLoadingController

	MassMatrix     mat.MatDense // 质量矩阵 M(q)
	StiffnessMatrix mat.MatDense // 刚度矩阵 K
	DampingMatrix   mat.MatDense // 阻尼矩阵 C

	CurrentState   SystemState
	CurrentForces  SystemForces
}

// NewDynamicsEngine 创建动力学引擎
func NewDynamicsEngine(
	bowLeft BowArmParams,
	bowRight BowArmParams,
	bowString BowStringParams,
	cam CamParams,
	pawl PawlRatchetParams,
	arrow ArrowParams,
	config SimulationConfig,
) *DynamicsEngine {
	eng := &DynamicsEngine{
		BowArmLeft:  bowLeft,
		BowArmRight: bowRight,
		BowString:   bowString,
		Cam:         cam,
		PawlRatchet: pawl,
		Arrow:       arrow,
		Config:      config,
	}

	eng.CamMechanism = NewCamMechanism(cam)
	eng.LoadingController = NewAutoLoadingController(eng.CamMechanism)

	eng.initMatrices()
	eng.initState()

	return eng
}

// initMatrices 初始化系统矩阵
// 系统自由度 n = 6
// q = [θ1, θ2, x_string, x_arrow, φ_cam, θ_ratchet]^T
func (de *DynamicsEngine) initMatrices() {
	n := 6

	// 质量矩阵 M (对角占优，广义质量)
	// M = diag([I1, I2, m_string, m_arrow, I_cam, I_ratchet])
	Mdata := make([]float64, n*n)
	Mdata[0*n+0] = de.BowArmLeft.MomentOfInertia   // I1
	Mdata[1*n+1] = de.BowArmRight.MomentOfInertia // I2
	Mdata[2*n+2] = de.BowString.MassPerUnit * de.BowString.Length0 // m_string
	Mdata[3*n+3] = de.Arrow.Mass                   // m_arrow
	Mdata[4*n+4] = 0.01 // I_cam (假设值，需根据实际凸轮几何计算)
	Mdata[5*n+5] = 0.005 // I_ratchet
	de.MassMatrix = *mat.NewDense(n, n, Mdata)

	// 刚度矩阵 K
	Kdata := make([]float64, n*n)
	Kdata[0*n+0] = de.calculateArmTorsionalStiffness() // 弩臂扭转刚度
	Kdata[1*n+1] = de.calculateArmTorsionalStiffness()
	Kdata[2*n+2] = de.BowString.Stiffness              // 弓弦线刚度
	de.StiffnessMatrix = *mat.NewDense(n, n, Kdata)

	// 阻尼矩阵 C (瑞利阻尼 C = αM + βK)
	α := 0.05 // 质量阻尼系数
	β := 0.01 // 刚度阻尼系数
	Cdata := make([]float64, n*n)
	for i := 0; i < n; i++ {
		Cdata[i*n+i] = α*Mdata[i*n+i] + β*Kdata[i*n+i]
	}
	de.DampingMatrix = *mat.NewDense(n, n, Cdata)
}

// initState 初始化系统状态
func (de *DynamicsEngine) initState() {
	n := 6

	// 初始广义位置
	q0 := mat.NewVecDense(n, []float64{
		0.1,    // θ1 左弩臂初始转角 [rad]（预弯角）
		-0.1,   // θ2 右弩臂初始转角 [rad]（对称）
		0.0,    // x_string 弓弦初始位移 [m]
		0.0,    // x_arrow 箭矢初始位移 [m]
		0.0,    // φ_cam 凸轮初始转角 [rad]
		0.0,    // θ_ratchet 棘轮初始转角 [rad]
	})

	// 初始广义速度
	qdot0 := mat.NewVecDense(n, nil) // 初始速度为0

	// 初始广义加速度
	qddot0 := mat.NewVecDense(n, nil)

	initialTension := de.CalculateBowStringTension(0.0)

	de.CurrentState = SystemState{
		StateVector: StateVector{
			Time:          0.0,
			Positions:     *q0,
			Velocities:    *qdot0,
			Accelerations: *qddot0,
		},
		LeftBowArm: BowArmState{
			Angle:         q0.AtVec(0),
			AngularVel:    qdot0.AtVec(0),
			AngularAcc:    qddot0.AtVec(0),
			BendingMoment: 0,
			ShearForce:    0,
			Stress:        0,
			Deflection:    0,
		},
		RightBowArm: BowArmState{
			Angle:         q0.AtVec(1),
			AngularVel:    qdot0.AtVec(1),
			AngularAcc:    qddot0.AtVec(1),
			BendingMoment: 0,
			ShearForce:    0,
			Stress:        0,
			Deflection:    0,
		},
		BowString: StringState{
			Tension:    initialTension,
			Elongation: 0,
			Strain:     0,
			Stress:     0,
		},
		Cam: CamFollowerState{
			Displacement:  0,
			Velocity:      0,
			Acceleration:  0,
			Jerk:          0,
			PressureAngle: 0,
		},
		Pawl: PawlState{
			State:         RatchetEngaged,
			Angle:         0,
			ContactForce:  ContactForce{},
			SpringForce:   0,
			ToothIndex:    0,
		},
		Arrow: ArrowState{
			Position:     *mat.NewVecDense(3, []float64{0.0, 0.0, 0.0}),
			Velocity:     *mat.NewVecDense(3, nil),
			Acceleration: *mat.NewVecDense(3, nil),
			InFlight:     false,
			Energy:       0.0,
		},
		Fatigue: FatigueState{
			CycleCount:   0,
			DamageSum:    0,
			MaxStress:    0,
			MinStress:    0,
			StressRatio:  0,
			LifeFraction: 0,
		},
	}

	// 初始化力向量
	de.CurrentForces = SystemForces{
		Elastic:    *mat.NewVecDense(n, nil),
		Damping:    *mat.NewVecDense(n, nil),
		Inertial:   *mat.NewVecDense(n, nil),
		Contact:    *mat.NewVecDense(n, nil),
		Gravity:    *mat.NewVecDense(n, nil),
		External:   *mat.NewVecDense(n, nil),
	}
}

// calculateArmTorsionalStiffness 计算弩臂扭转刚度
// 基于悬臂梁弯曲理论:
// k_t = EI / L
// 其中: I = bh³/12 (矩形截面惯性矩)
//      E - 弹性模量, b - 宽度, h - 厚度, L - 长度
func (de *DynamicsEngine) calculateArmTorsionalStiffness() float64 {
	E := de.BowArmLeft.Material.YoungsModulus
	b := de.BowArmLeft.Width
	h := de.BowArmLeft.Thickness
	L := de.BowArmLeft.Length

	// 截面惯性矩 I = bh³/12
	I := b * h * h * h / 12.0

	// 扭转刚度 k_t = EI / L [N·m/rad]
	return E * I / L
}

// CalculateBowStringTension 计算弓弦张力
// 考虑材料非线性的胡克定律:
// T(ΔL) = kΔL + αk(ΔL)³
// 其中: k - 线刚度, α - 非线性系数
// 当 ΔL ≤ 0 时, T = 0 (弓弦松弛)
func (de *DynamicsEngine) CalculateBowStringTension(ΔL float64) float64 {
	if ΔL <= 0 {
		return 0.0 // 弓弦松弛，无张力
	}

	k := de.BowString.Stiffness
	α := de.BowString.NonlinearCoeff

	// 非线性胡克定律: T = kΔL + αk(ΔL)³
	T := k*ΔL + α*k*ΔL*ΔL*ΔL

	// 限制最大张力
	return math.Min(T, de.BowString.MaxTension)
}

// CalculateBowArmDynamics 计算弩臂角运动方程
// 基于欧拉-拉格朗日方程: d/dt(∂L/∂q̇) - ∂L/∂q = Q
// 拉格朗日量 L = T - V
// 动能 T = ½Iθ̇², 势能 V = ½k_tθ² + mgL_cm(1-cosθ)
//
// 运动方程:
// Iθ̈ + cθ̇ + k_tθ + T*L*cos(θ) + mgL_cm*sin(θ) = M_external
// 其中:
//   I - 转动惯量
//   c - 阻尼系数
//   k_t - 扭转刚度
//   T - 弓弦张力
//   L - 弩臂长度
//   m - 弩臂质量
//   L_cm - 质心位置 (L/2)
func (de *DynamicsEngine) CalculateBowArmDynamics(
	angle float64,
	angularVel float64,
	arm BowArmParams,
	stringTension float64,
) (float64, float64, float64) {
	I := arm.MomentOfInertia
	c := arm.DampingCoeff
	k_t := de.calculateArmTorsionalStiffness()
	L := arm.Length
	m := arm.Mass
	g := de.Config.Gravity
	L_cm := L / 2.0

	// 弹性恢复力矩: M_k = k_t * θ
	M_elastic := k_t * angle

	// 阻尼力矩: M_damp = c * θ̇
	M_damping := c * angularVel

	// 重力力矩: M_grav = mgL_cm * sin(θ)
	M_gravity := m * g * L_cm * math.Sin(angle)

	// 弓弦张力力矩: M_string = T * L * cos(θ)
	// 张力作用在弩臂端点，力臂为 L*cos(θ)
	M_string := stringTension * L * math.Cos(angle)

	// 总力矩平衡: Iθ̈ = -M_elastic - M_damping - M_gravity - M_string
	// 角加速度: θ̈ = -(M_elastic + M_damping + M_gravity + M_string) / I
	angularAcc := -(M_elastic + M_damping + M_gravity + M_string) / I

	// 计算弩臂根部弯矩
	// M(x) = T * (L - x) * cos(θ) + mg * (L_cm - x) * sin(θ)
	// 根部弯矩 (x=0): M_root = T*L*cos(θ) + mg*L_cm*sin(θ)
	bendingMoment := stringTension*L*math.Cos(angle) + m*g*L_cm*math.Sin(angle)

	// 计算弯曲应力 (悬臂梁根部)
	// σ = M * y_max / I, y_max = h/2
	h := arm.Thickness
	b := arm.Width
	I_section := b * h * h * h / 12.0
	y_max := h / 2.0
	stress := bendingMoment * y_max / I_section

	return angularAcc, bendingMoment, stress
}

// CalculateContactForce 计算接触力
// 基于Hertz接触理论 + 库仑摩擦模型
//
// 法向力 (Hertz模型):
// Fn = k_c * δ^(3/2) + c_c * δ̇ * δ^(1/2), 当 δ > 0
// Fn = 0, 当 δ ≤ 0
// 其中: k_c - 接触刚度, c_c - 接触阻尼, δ - 穿透深度
//
// 切向摩擦力 (库仑摩擦):
// Ff = -μ * Fn * sign(v_rel), 当 |v_rel| > v_threshold
// Ff = -μ * Fn * (v_rel / v_threshold), 当 |v_rel| ≤ v_threshold (Stribeck润滑)
func (de *DynamicsEngine) CalculateContactForce(
	penetration float64,
	penetrationVel float64,
	relativeVel float64,
	contactStiffness float64,
	contactDamping float64,
	mu float64,
) ContactForce {
	var Fn float64
	threshold := 1e-4 // 速度阈值 [m/s]

	if penetration > 0 {
		// Hertz法向接触力
		sqrtDelta := math.Sqrt(penetration)
		Fn = contactStiffness*penetration*sqrtDelta + contactDamping*penetrationVel*sqrtDelta
		Fn = math.Max(0.0, Fn) // 只能有压力
	} else {
		Fn = 0.0
	}

	// 库仑摩擦力
	var Ff float64
	if math.Abs(relativeVel) > threshold {
		Ff = -mu * Fn * math.Copysign(1.0, relativeVel)
	} else {
		// 低速度时的平滑过渡（避免数值振荡）
		Ff = -mu * Fn * (relativeVel / threshold)
	}

	return ContactForce{
		Normal:      *mat.NewVecDense(2, []float64{0.0, Fn}),
		Friction:    *mat.NewVecDense(2, []float64{Ff, 0.0}),
		Penetration: penetration,
	}
}

// UpdatePawlRatchetState 更新棘爪棘轮机构状态机
// 状态转移:
//   Engaged → Disengaging → Freewheeling → Engaging → Engaged
//
// 状态判定条件:
// - Engaged: 棘爪啮入齿槽，棘轮被锁定
// - Disengaging: 棘爪受外力抬起，开始脱离
// - Freewheeling: 棘爪完全脱离，棘轮自由转动
// - Engaging: 棘爪在弹簧力作用下下落，准备啮入下一个齿槽
func (de *DynamicsEngine) UpdatePawlRatchetState(
	pawlAngle float64,
	ratchetAngle float64,
	ratchetVel float64,
	pawlCommand int,
) PawlState {
	params := de.PawlRatchet
	currentState := de.CurrentState.Pawl.State

	toothPitch := 2 * math.Pi / float64(params.ToothCount) // 齿距角
	engagedThreshold := toothPitch * 0.1                  // 啮合判定阈值
	disengagedThreshold := toothPitch * 0.8               // 脱离判定阈值

	// 计算当前齿位置索引
	toothIndex := int(math.Mod(ratchetAngle, 2*math.Pi) / toothPitch)

	// 计算弹簧力: F_spring = F0 + k_p * θ_p
	springForce := params.Preload + params.SpringStiffness*pawlAngle

	// 状态机逻辑
	var newState RatchetState

	switch currentState {
	case RatchetEngaged:
		if pawlCommand < 0 || pawlAngle > disengagedThreshold {
			newState = RatchetDisengaging
		} else {
			newState = RatchetEngaged
		}

	case RatchetDisengaging:
		if pawlAngle > disengagedThreshold {
			newState = RatchetFreewheeling
		} else if pawlCommand > 0 && pawlAngle < engagedThreshold {
			newState = RatchetEngaging
		} else {
			newState = RatchetDisengaging
		}

	case RatchetFreewheeling:
		if pawlCommand > 0 {
			newState = RatchetEngaging
		} else {
			newState = RatchetFreewheeling
		}

	case RatchetEngaging:
		if pawlAngle < engagedThreshold {
			newState = RatchetEngaged
		} else if pawlCommand < 0 {
			newState = RatchetDisengaging
		} else {
			newState = RatchetEngaging
		}
	}

	// 计算接触力（仅在啮合和啮入状态）
	var contactForce ContactForce
	if newState == RatchetEngaged || newState == RatchetEngaging {
		// 估算穿透深度
		penetration := math.Max(0, engagedThreshold-pawlAngle)
		penetrationVel := -ratchetVel * params.PawlLength * math.Sin(pawlAngle)

		// 相对滑动速度
		relativeVel := ratchetVel * params.PawlLength * math.Cos(pawlAngle)

		contactForce = de.CalculateContactForce(
			penetration,
			penetrationVel,
			relativeVel,
			1e8,  // 接触刚度
			1e3,  // 接触阻尼
			params.FrictionCoeff,
		)
	}

	return PawlState{
		State:        newState,
		Angle:        pawlAngle,
		ContactForce: contactForce,
		SpringForce:  springForce,
		ToothIndex:   toothIndex,
	}
}

// ComputeMassMatrix 计算质量矩阵 M(q)
// 对于大变形问题，质量矩阵可能与广义位置相关
// 本实现假设小变形，质量矩阵为常数矩阵
func (de *DynamicsEngine) ComputeMassMatrix(q mat.VecDense) mat.MatDense {
	return de.MassMatrix
}

// ComputeGeneralizedForces 计算广义力向量 Q(q, q̇, t)
// Q = Q_elastic + Q_damping + Q_contact + Q_gravity + Q_external
func (de *DynamicsEngine) ComputeGeneralizedForces(
	q mat.VecDense,
	qdot mat.VecDense,
	t float64,
) mat.VecDense {
	n := q.Len()
	Q := mat.NewVecDense(n, nil)

	// 提取广义坐标
	θ1 := q.AtVec(0)   // 左弩臂转角
	θ2 := q.AtVec(1)   // 右弩臂转角
	xs := q.AtVec(2)   // 弓弦位移
	xa := q.AtVec(3)   // 箭矢位移
	φ := q.AtVec(4)    // 凸轮转角
	θr := q.AtVec(5)   // 棘轮转角

	θ1dot := qdot.AtVec(0)
	θ2dot := qdot.AtVec(1)
	xsdot := qdot.AtVec(2)
	φdot := qdot.AtVec(4)

	// 1. 计算弓弦伸长量（几何关系）
	// 弓弦连接左右弩臂端点，长度随弩臂转角变化
	// L_string(θ1, θ2) = sqrt( (x2-x1)² + (y2-y1)² )
	// 其中: (x1,y1) = 左端点坐标, (x2,y2) = 右端点坐标
	L1 := de.BowArmLeft.Length
	L2 := de.BowArmRight.Length
	pivot1 := de.BowArmLeft.PivotPoint
	pivot2 := de.BowArmRight.PivotPoint

	x1 := pivot1.AtVec(0) + L1*math.Sin(θ1)
	y1 := pivot1.AtVec(1) + L1*math.Cos(θ1)
	x2 := pivot2.AtVec(0) + L2*math.Sin(θ2)
	y2 := pivot2.AtVec(1) + L2*math.Cos(θ2)

	currentStringLength := math.Sqrt((x2-x1)*(x2-x1) + (y2-y1)*(y2-y1))
	ΔL := currentStringLength - de.BowString.Length0 + xs

	// 弓弦张力
	T := de.CalculateBowStringTension(ΔL)

	// 2. 计算弩臂动力学
	acc1, _, stress1 := de.CalculateBowArmDynamics(θ1, θ1dot, de.BowArmLeft, T)
	acc2, _, stress2 := de.CalculateBowArmDynamics(θ2, θ2dot, de.BowArmRight, T)

	// 3. 计算凸轮从动件运动
	camState := de.CamMechanism.CalculateFollowerMotion(φ, φdot)

	// 4. 计算广义力
	// 弹性力（弹簧力）
	Q.SetVec(0, -de.calculateArmTorsionalStiffness()*θ1) // 左弩臂弹性力矩
	Q.SetVec(1, -de.calculateArmTorsionalStiffness()*θ2) // 右弩臂弹性力矩
	Q.SetVec(2, -de.BowString.Stiffness*xs)              // 弓弦弹性力

	// 弓弦张力对弩臂的力矩
	// 张力在端点的作用力通过几何关系转换为广义力
	// ∂L/∂θ1 是弓弦长度对 θ1 的偏导数
	dL_dθ1 := L1 * (-(x2 - x1) * math.Cos(θ1) + (y2 - y1) * math.Sin(θ1)) / currentStringLength
	dL_dθ2 := L2 * ((x2 - x1) * math.Cos(θ2) - (y2 - y1) * math.Sin(θ2)) / currentStringLength

	// 张力产生的广义力: Q_T = -T * (∂L/∂q)
	if ΔL > 0 {
		Q.SetVec(0, Q.AtVec(0) - T*dL_dθ1)
		Q.SetVec(1, Q.AtVec(1) - T*dL_dθ2)
		Q.SetVec(2, Q.AtVec(2) - T) // 弓弦张力直接作用在 x_string 方向
	}

	// 阻尼力（瑞利阻尼）
	var dampingForce mat.VecDense
	dampingForce.MulVec(&de.DampingMatrix, &qdot)
	Q.SubVec(Q, &dampingForce)

	// 保存弩臂状态（用于输出）
	de.CurrentState.LeftBowArm = BowArmState{
		Angle:      θ1,
		AngularVel: θ1dot,
		AngularAcc: acc1,
		Stress:     stress1,
	}
	de.CurrentState.RightBowArm = BowArmState{
		Angle:      θ2,
		AngularVel: θ2dot,
		AngularAcc: acc2,
		Stress:     stress2,
	}

	// 保存弓弦状态
	ε := ΔL / de.BowString.Length0 // 应变
	σ := T * de.BowString.Stiffness / de.BowString.Length0 * de.BowString.Length0 // 应力简化
	de.CurrentState.BowString = StringState{
		Tension:    T,
		Elongation: ΔL,
		Strain:     ε,
		Stress:     σ,
	}

	// 保存凸轮状态
	de.CurrentState.Cam = camState

	return *Q
}

// StateDerivative 计算状态导数 f(q, q̇, t)
// 对于二阶系统 M(q)q̈ = Q(q, q̇, t)
// 定义状态向量 y = [q; q̇], 则 ẏ = [q̇; M^(-1)Q]
func (de *DynamicsEngine) StateDerivative(y mat.VecDense, t float64) mat.VecDense {
	n := y.Len() / 2

	// 提取 q 和 q̇
	q := *mat.NewVecDense(n, y.RawVector().Data[:n])
	qdot := *mat.NewVecDense(n, y.RawVector().Data[n:])

	// 计算广义力
	Q := de.ComputeGeneralizedForces(q, qdot, t)

	// 求解 M q̈ = Q → q̈ = M^(-1) Q
	var qddot mat.VecDense
	var MInv mat.Dense
	err := MInv.Inverse(&de.MassMatrix)
	if err != nil {
		// 质量矩阵奇异，使用伪逆或对角近似
		qddot = *mat.NewVecDense(n, nil)
	} else {
		qddot.MulVec(&MInv, &Q)
	}

	// 构造状态导数 ẏ = [q̇; q̈]
	ydot := mat.NewVecDense(2*n, nil)
	for i := 0; i < n; i++ {
		ydot.SetVec(i, qdot.AtVec(i))
		ydot.SetVec(n+i, qddot.AtVec(i))
	}

	return *ydot
}

// RK4Step 四阶龙格-库塔积分步
// 对于 ẏ = f(t, y)
// k1 = Δt * f(t, y)
// k2 = Δt * f(t + Δt/2, y + k1/2)
// k3 = Δt * f(t + Δt/2, y + k2/2)
// k4 = Δt * f(t + Δt, y + k3)
// y_new = y + (k1 + 2k2 + 2k3 + k4) / 6
//
// RK4的局部截断误差为 O(Δt^5)，全局截断误差为 O(Δt^4)
// 是工程仿真中最常用的显式积分方法之一
func (de *DynamicsEngine) RK4Step(y mat.VecDense, t float64, dt float64) mat.VecDense {
	n := y.Len()

	// k1 = f(t, y)
	k1 := de.StateDerivative(y, t)
	k1.ScaleVec(dt, &k1)

	// k2 = f(t + dt/2, y + k1/2)
	y2 := mat.NewVecDense(n, nil)
	y2.AddScaledVec(&y, 0.5, &k1)
	k2 := de.StateDerivative(*y2, t+dt/2)
	k2.ScaleVec(dt, &k2)

	// k3 = f(t + dt/2, y + k2/2)
	y3 := mat.NewVecDense(n, nil)
	y3.AddScaledVec(&y, 0.5, &k2)
	k3 := de.StateDerivative(*y3, t+dt/2)
	k3.ScaleVec(dt, &k3)

	// k4 = f(t + dt, y + k3)
	y4 := mat.NewVecDense(n, nil)
	y4.AddVec(&y, &k3)
	k4 := de.StateDerivative(*y4, t+dt)
	k4.ScaleVec(dt, &k4)

	// y_new = y + (k1 + 2k2 + 2k3 + k4) / 6
	yNew := mat.NewVecDense(n, nil)
	yNew.AddVec(&y, &k1)
	yNew.AddScaledVec(yNew, 2.0, &k2)
	yNew.AddScaledVec(yNew, 2.0, &k3)
	yNew.AddVec(yNew, &k4)
	yNew.ScaleVec(1.0/6.0, yNew)
	yNew.AddVec(&y, yNew)

	return *yNew
}

// Step 执行一步仿真
func (de *DynamicsEngine) Step(dt float64) SystemState {
	n := 6

	// 构造状态向量 y = [q; q̇]
	y := mat.NewVecDense(2*n, nil)
	for i := 0; i < n; i++ {
		y.SetVec(i, de.CurrentState.Positions.AtVec(i))
		y.SetVec(n+i, de.CurrentState.Velocities.AtVec(i))
	}

	// RK4积分
	t := de.CurrentState.Time
	yNew := de.RK4Step(*y, t, dt)

	// 提取新状态
	qNew := *mat.NewVecDense(n, yNew.RawVector().Data[:n])
	qdotNew := *mat.NewVecDense(n, yNew.RawVector().Data[n:])

	// 计算加速度
	Q := de.ComputeGeneralizedForces(qNew, qdotNew, t+dt)
	var qddotNew mat.VecDense
	var MInv mat.Dense
	err := MInv.Inverse(&de.MassMatrix)
	if err == nil {
		qddotNew.MulVec(&MInv, &Q)
	} else {
		qddotNew = *mat.NewVecDense(n, nil)
	}

	// 更新状态
	de.CurrentState.Time = t + dt
	de.CurrentState.Positions = qNew
	de.CurrentState.Velocities = qdotNew
	de.CurrentState.Accelerations = qddotNew

	// 更新能量
	de.updateEnergy()

	return de.CurrentState
}

// updateEnergy 计算系统能量
// 总能量 E = Ek + Ep
// 动能 Ek = ½q̇^T M q̇
// 势能 Ep = ½q^T K q + V_gravity
// 耗散能 Ed = ∫ q̇^T C q̇ dt
func (de *DynamicsEngine) updateEnergy() {
	n := 6
	q := de.CurrentState.Positions
	qdot := de.CurrentState.Velocities

	// 动能 Ek = ½ q̇^T M q̇
	var Mv mat.VecDense
	Mv.MulVec(&de.MassMatrix, &qdot)
	Ek := 0.5 * mat.Dot(&qdot, &Mv)

	// 势能 Ep = ½ q^T K q
	var Kv mat.VecDense
	Kv.MulVec(&de.StiffnessMatrix, &q)
	Ep := 0.5 * mat.Dot(&q, &Kv)

	// 重力势能（弩臂）
	m1 := de.BowArmLeft.Mass
	m2 := de.BowArmRight.Mass
	L1 := de.BowArmLeft.Length
	L2 := de.BowArmRight.Length
	g := de.Config.Gravity

	Vg1 := m1 * g * L1 / 2.0 * (1 - math.Cos(q.AtVec(0)))
	Vg2 := m2 * g * L2 / 2.0 * (1 - math.Cos(q.AtVec(1)))
	Ep += Vg1 + Vg2

	// 瞬时耗散功率 Pd = q̇^T C q̇
	var Cv mat.VecDense
	Cv.MulVec(&de.DampingMatrix, &qdot)
	Pd := mat.Dot(&qdot, &Cv)

	de.CurrentState.KineticEnergy = Ek
	de.CurrentState.PotentialEnergy = Ep
	de.CurrentState.TotalEnergy = Ek + Ep
	de.CurrentState.DissipatedEnergy += Pd * de.Config.TimeStep
}
