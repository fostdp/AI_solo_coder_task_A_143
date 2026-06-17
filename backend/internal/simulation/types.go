package simulation

import "gonum.org/v1/gonum/mat"

// MaterialParams 材料参数
type MaterialParams struct {
	YoungsModulus float64 // 弹性模量 E [Pa]
	YieldStrength float64 // 屈服强度 σ_y [Pa]
	Density       float64 // 密度 ρ [kg/m³]
	PoissonsRatio float64 // 泊松比 ν
}

// BowArmParams 弩臂参数
type BowArmParams struct {
	Length        float64       // 弩臂长度 L [m]
	Width         float64       // 弩臂宽度 b [m]
	Thickness     float64       // 弩臂厚度 h [m]
	Mass          float64       // 弩臂质量 m [kg]
	MomentOfInertia float64     // 转动惯量 I [kg·m²]
	PivotPoint    mat.VecDense  // 枢轴点坐标 [x, y] [m]
	Material      MaterialParams
	DampingCoeff  float64       // 阻尼系数 c [N·s/m]
}

// BowStringParams 弓弦参数
type BowStringParams struct {
	Length0       float64       // 原长 L0 [m]
	Stiffness     float64       // 线刚度 k [N/m]
	NonlinearCoeff float64      // 非线性系数 α
	MassPerUnit   float64       // 线密度 ρ_l [kg/m]
	PreTension    float64       // 预紧力 T0 [N]
	DampingCoeff  float64       // 阻尼系数 c [N·s/m]
	MaxTension    float64       // 最大张力 T_max [N]
	Material      MaterialParams // 材料参数
}

// CamParams 凸轮参数
type CamParams struct {
	BaseRadius    float64       // 基圆半径 Rb [m]
	RollerRadius  float64       // 滚子半径 Rr [m]
	PressureAngle float64       // 压力角 α_max [rad]
	Lift          float64       // 升程 h [m]
	RotationSpeed float64       // 转速 ω [rad/s]
	PhaseAngle    float64       // 相位角 φ0 [rad]
	FrictionCoeff float64       // 摩擦系数 μ
	RotSpeed      float64       // 转速 (兼容字段) [rad/s]
}

// BufferSpringParams 缓冲弹簧参数
// 用于消除凸轮机构高速运动时的冲击，保证从动件与凸轮始终接触
type BufferSpringParams struct {
	Stiffness       float64 // 弹簧刚度 k_buffer [N/m]
	Preload         float64 // 预紧力 F0 [N]，保证零速时仍有接触压力
	Damping         float64 // 阻尼系数 c_buffer [N·s/m]，吸收冲击能量
	MaxCompression  float64 // 最大压缩量 δ_max [m]
	EquivalentMass  float64 // 等效质量 meq [kg]，用于弹簧-质量系统
}

// PawlRatchetParams 棘爪棘轮参数
type PawlRatchetParams struct {
	NumTeeth           int           // 齿数 Z
	ToothCount         int           // 齿数 Z (兼容字段)
	ToothAngle         float64       // 齿顶角 β [rad]
	PawlLength         float64       // 棘爪长度 Lp [m]
	PawlMass           float64       // 棘爪质量 [kg]
	RatchetMass        float64       // 棘轮质量 [kg]
	TorsionalStiffness float64       // 扭转弹簧刚度 [N·m/rad]
	SpringStiffness    float64       // 弹簧刚度 kp [N/m]
	Preload            float64       // 预紧力 F0 [N]
	FrictionCoeff      float64       // 摩擦系数 μ
}

// ArrowParams 箭矢参数
type ArrowParams struct {
	Mass          float64       // 质量 m_arrow [kg]
	Length        float64       // 长度 L_arrow [m]
	Diameter      float64       // 直径 d [m]
	DragCoeff     float64       // 阻力系数 Cd
	// 0.47 for sphere, ~0.05 for streamlined
}

// StateVector 系统状态向量
// q = [θ1, θ2, x_string, x_arrow, φ_cam, θ_ratchet]^T
// 其中: θ1, θ2 - 左右弩臂转角, x_string - 弓弦位移, x_arrow - 箭矢位移
//       φ_cam - 凸轮转角, θ_ratchet - 棘轮转角
type StateVector struct {
	Time          float64       // 当前时间 t [s]
	Positions     mat.VecDense  // 广义位置 q [rad, rad, m, m, rad, rad]
	Velocities    mat.VecDense  // 广义速度 q̇ [rad/s, rad/s, m/s, m/s, rad/s, rad/s]
	Accelerations mat.VecDense  // 广义加速度 q̈ [rad/s², ...]
}

// ContactForce 接触力
type ContactForce struct {
	Normal        mat.VecDense  // 法向力 Fn [N]
	Friction      mat.VecDense  // 摩擦力 Ff [N]
	ContactPoint  mat.VecDense  // 接触点坐标 [m]
	Penetration   float64       // 穿透深度 δ [m]
}

// BowArmState 弩臂状态
type BowArmState struct {
	Angle         float64       // 转角 θ [rad]
	AngularVel    float64       // 角速度 ω [rad/s]
	AngularAcc    float64       // 角加速度 α [rad/s²]
	BendingMoment float64       // 弯矩 M [N·m]
	ShearForce    float64       // 剪力 V [N]
	Stress        float64       // 弯曲应力 σ [Pa]
}

// CamFollowerState 凸轮从动件状态
type CamFollowerState struct {
	Displacement    float64       // 位移 s [m]
	Velocity        float64       // 速度 v [m/s]
	Acceleration    float64       // 加速度 a [m/s²]
	Jerk            float64       // 跃度 j [m/s³]
	PressureAngle   float64       // 压力角 α [rad]
	SpringForce     float64       // 缓冲弹簧力 F_spring [N]
	SpringDeflection float64      // 弹簧变形量 δ [m]
	ContactForce    float64       // 接触力 Fn [N]
	IsContacting    bool          // 是否保持接触
	ImpactVelocity  float64       // 冲击速度 v_impact [m/s]
}

// StringState 弓弦状态
type StringState struct {
	Tension       float64       // 张力 T [N]
	Elongation    float64       // 伸长量 ΔL [m]
	Strain        float64       // 应变 ε
	Stress        float64       // 应力 σ [Pa]
}

// ArrowState 箭矢状态
type ArrowState struct {
	Position      mat.VecDense  // 位置矢量 r [m]
	Velocity      mat.VecDense  // 速度矢量 v [m/s]
	Acceleration  mat.VecDense  // 加速度矢量 a [m/s²]
	InFlight      bool          // 是否在飞行中
	Energy        float64       // 动能 Ek [J]
}

// FatigueState 疲劳状态
type FatigueState struct {
	CycleCount    float64       // 循环次数 N
	DamageSum     float64       // 损伤累积 Σ(ni/Ni)
	MaxStress     float64       // 最大应力 σ_max [Pa]
	MinStress     float64       // 最小应力 σ_min [Pa]
	StressRatio   float64       // 应力比 R = σ_min/σ_max
	LifeFraction  float64       // 寿命损耗比例
}

// RatchetState 棘轮机构状态
type RatchetState int

const (
	RatchetEngaged RatchetState = iota // 棘爪啮合，锁定
	RatchetDisengaging                 // 棘爪脱离中
	RatchetFreewheeling                // 自由转动
	RatchetEngaging                    // 棘爪啮入中
)

// PawlState 棘爪状态
type PawlState struct {
	State         RatchetState  // 状态机状态
	Angle         float64       // 棘爪转角 [rad]
	ContactForce  ContactForce  // 接触力
	SpringForce   float64       // 弹簧力 [N]
	ToothIndex    int           // 当前齿索引
}

// SystemState 完整系统状态
type SystemState struct {
	StateVector
	LeftBowArm    BowArmState
	RightBowArm   BowArmState
	BowString     StringState
	Cam           CamFollowerState
	Pawl          PawlState
	Arrow         ArrowState
	Fatigue       FatigueState
	TotalEnergy   float64       // 系统总能量 E [J]
	KineticEnergy float64       // 动能 Ek [J]
	PotentialEnergy float64     // 势能 Ep [J]
	DissipatedEnergy float64    // 耗散能 Ed [J]
}

// SimulationConfig 仿真配置
type SimulationConfig struct {
	TimeStep        float64       // 时间步长 Δt [s]
	TotalTime       float64       // 总仿真时间 T [s]
	Gravity         float64       // 重力加速度 g [m/s²]
	AirDensity      float64       // 空气密度 ρ_air [kg/m³]
	SolverType      string        // 求解器类型 ("RK4", "Euler", etc.)
	SaveInterval    int           // 保存间隔（步数）
	SpeedMultiplier float64       // 仿真速度倍率
}

// SystemForces 系统广义力
type SystemForces struct {
	Elastic       mat.VecDense  // 弹性力 F_spring [N, N·m]
	Damping       mat.VecDense  // 阻尼力 F_damper [N, N·m]
	Inertial      mat.VecDense  // 惯性力 F_inertial
	Contact       mat.VecDense  // 接触力 F_contact
	Gravity       mat.VecDense  // 重力 F_gravity
	External      mat.VecDense  // 外力 F_external
}

// CamProfilePoint 凸轮轮廓点
type CamProfilePoint struct {
	Angle         float64       // 凸轮转角 φ [rad]
	Radius        float64       // 向径 r(φ) [m]
	X             float64       // X坐标 [m]
	Y             float64       // Y坐标 [m]
	NormalX       float64       // 法向X分量
	NormalY       float64       // 法向Y分量
	Curvature     float64       // 曲率 κ [1/m]
}

// LoadingSequence 装填时序
type LoadingSequence struct {
	Phase         int           // 阶段
	StartTime     float64       // 开始时间 [s]
	Duration      float64       // 持续时间 [s]
	Description   string        // 描述
	Completed     bool          // 是否完成
}

// SimulationResult 仿真结果
type SimulationResult struct {
	Config        SimulationConfig
	States        []SystemState
	FinalState    SystemState
	ArrowFlight   []ArrowState
	MaxValues     map[string]float64
	FatigueResult FatigueState
}
