package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Redis      RedisConfig      `mapstructure:"redis"`
	Simulation SimulationConfig `mapstructure:"simulation"`
	Alert      AlertConfig      `mapstructure:"alert"`
	WebSocket  WebSocketConfig  `mapstructure:"websocket"`
}

type ServerConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`
}

type DatabaseConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	User         string `mapstructure:"user"`
	Password     string `mapstructure:"password"`
	DBName       string `mapstructure:"dbname"`
	SSLMode      string `mapstructure:"sslmode"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type SimulationConfig struct {
	TimeStep            float64 `mapstructure:"time_step"`
	SpeedMultiplier     float64 `mapstructure:"speed_multiplier"`
	EnableRLOptimization bool   `mapstructure:"enable_rl_optimization"`
}

type AlertConfig struct {
	CheckInterval  int `mapstructure:"check_interval"`
	CooldownPeriod int `mapstructure:"cooldown_period"`
}

type WebSocketConfig struct {
	PingInterval int `mapstructure:"ping_interval"`
	PongTimeout  int `mapstructure:"pong_timeout"`
	WriteWait    int `mapstructure:"write_wait"`
}

// MechanismParams 机构动力学参数（从mechanism_params.json加载）
type MechanismParams struct {
	BowArm       BowArmParams       `json:"bow_arm"`
	BowString    BowStringParams    `json:"bow_string"`
	Cam          CamParams          `json:"cam"`
	BufferSpring BufferSpringParams `json:"buffer_spring"`
	PawlRatchet  PawlRatchetParams  `json:"pawl_ratchet"`
	Arrow        ArrowParams        `json:"arrow"`
	Magazine     MagazineParams     `json:"magazine"`
	DesignSpec   DesignSpecParams   `json:"design_spec"`
	Simulation   SimEnvParams       `json:"simulation"`
}

type BowArmParams struct {
	Length           float64 `json:"length"`
	Width            float64 `json:"width"`
	Thickness        float64 `json:"thickness"`
	Mass             float64 `json:"mass"`
	MomentOfInertia  float64 `json:"moment_of_inertia"`
	YoungsModulus    float64 `json:"youngs_modulus"`
	MaxStress        float64 `json:"max_stress"`
	DampingCoeff     float64 `json:"damping_coeff"`
	TorsionalStiffness float64 `json:"torsional_stiffness"`
}

type BowStringParams struct {
	Length0                float64 `json:"length0"`
	Radius                 float64 `json:"radius"`
	YoungsModulus          float64 `json:"youngs_modulus"`
	YieldStrength          float64 `json:"yield_strength"`
	FatigueStrengthCoeff   float64 `json:"fatigue_strength_coeff"`
	FatigueStrengthExponent float64 `json:"fatigue_strength_exponent"`
	PreTension             float64 `json:"pre_tension"`
	DampingCoeff           float64 `json:"damping_coeff"`
	Material               string  `json:"material"`
	NonlinearCoeff         float64 `json:"nonlinear_coeff"`
}

type CamParams struct {
	BaseRadius    float64 `json:"base_radius"`
	RollerRadius  float64 `json:"roller_radius"`
	PressureAngle float64 `json:"pressure_angle"`
	Lift          float64 `json:"lift"`
	RotationSpeed float64 `json:"rotation_speed"`
	PhaseAngle    float64 `json:"phase_angle"`
	FrictionCoeff float64 `json:"friction_coeff"`
	RotSpeed      float64 `json:"rot_speed"`
}

type BufferSpringParams struct {
	Stiffness      float64 `json:"stiffness"`
	Preload        float64 `json:"preload"`
	Damping        float64 `json:"damping"`
	MaxCompression float64 `json:"max_compression"`
	EquivalentMass float64 `json:"equivalent_mass"`
}

type PawlRatchetParams struct {
	NumTeeth       int     `json:"num_teeth"`
	PawlMass       float64 `json:"pawl_mass"`
	PawlLength     float64 `json:"pawl_length"`
	SpringStiffness float64 `json:"spring_stiffness"`
	Damping        float64 `json:"damping"`
	FrictionCoeff  float64 `json:"friction_coeff"`
	RatchetRadius  float64 `json:"ratchet_radius"`
	ToothAngle     float64 `json:"tooth_angle"`
}

type ArrowParams struct {
	Mass      float64 `json:"mass"`
	Length    float64 `json:"length"`
	Radius    float64 `json:"radius"`
	DragCoeff float64 `json:"drag_coeff"`
	TipMass   float64 `json:"tip_mass"`
}

type MagazineParams struct {
	Capacity        int     `json:"capacity"`
	ReloadTime      float64 `json:"reload_time"`
	JamProbability  float64 `json:"jam_probability"`
}

type DesignSpecParams struct {
	MaxTension       float64 `json:"max_tension"`
	DesignFireRate   float64 `json:"design_fire_rate"`
	MaxDeformation   float64 `json:"max_deformation"`
	CriticalFatigue  float64 `json:"critical_fatigue"`
}

type SimEnvParams struct {
	Dt             float64 `json:"dt"`
	Gravity        float64 `json:"gravity"`
	AirDensity     float64 `json:"air_density"`
	SpeedMultiplier float64 `json:"speed_multiplier"`
}

// RLParams 强化学习参数（从rl_params.json加载）
type RLParams struct {
	Agent        AgentConfig        `json:"agent"`
	Training     TrainingConfig     `json:"training"`
	Pretrain     PretrainConfig     `json:"pretrain"`
	ExpertPolicy ExpertPolicyConfig `json:"expert_policy"`
	Exploration  ExplorationConfig  `json:"exploration"`
	Logging      LoggingConfig      `json:"logging"`
}

type AgentConfig struct {
	StateDimension     int     `json:"state_dimension"`
	ActionDimension    int     `json:"action_dimension"`
	ReplayBufferSize   int     `json:"replay_buffer_size"`
	BatchSize          int     `json:"batch_size"`
	Gamma              float64 `json:"gamma"`
	EpsilonStart       float64 `json:"epsilon_start"`
	EpsilonEnd         float64 `json:"epsilon_end"`
	EpsilonDecay       float64 `json:"epsilon_decay"`
	LearningRate       float64 `json:"learning_rate"`
	TargetUpdateFreq   int     `json:"target_update_freq"`
	GradientClipNorm   float64 `json:"gradient_clip_norm"`
	HiddenLayerSize    int     `json:"hidden_layer_size"`
	NumLayers          int     `json:"num_layers"`
}

type TrainingConfig struct {
	MaxEpisodes         int     `json:"max_episodes"`
	MaxStepsPerEpisode  int     `json:"max_steps_per_episode"`
	ConvergenceWindow   int     `json:"convergence_window"`
	ConvergenceThreshold float64 `json:"convergence_threshold"`
	FireRateWeight      float64 `json:"fire_rate_weight"`
	FatiguePenalty      float64 `json:"fatigue_penalty"`
	LowFireRatePenalty  float64 `json:"low_fire_rate_penalty"`
	MinFireRate         float64 `json:"min_fire_rate"`
	FatigueThreshold    float64 `json:"fatigue_threshold"`
	BaseLoadingInterval float64 `json:"base_loading_interval"`
	OverfatiguePenalty  float64 `json:"overfatigue_penalty"`
	EmptyMagazinePenalty float64 `json:"empty_magazine_penalty"`
	FireRateIncreaseReward float64 `json:"fire_rate_increase_reward"`
}

type PretrainConfig struct {
	EnablePretrain               bool    `json:"enable_pretrain"`
	PretrainEpisodes             int     `json:"pretrain_episodes"`
	PretrainEpochs               int     `json:"pretrain_epochs"`
	PretrainLearningRateMultiplier float64 `json:"pretrain_learning_rate_multiplier"`
	ExpertActionTargetValue      float64 `json:"expert_action_target_value"`
	OtherActionTargetValue       float64 `json:"other_action_target_value"`
	PretrainEpsilonReduction     float64 `json:"pretrain_epsilon_reduction"`
}

type ExpertPolicyConfig struct {
	HighFatigueThreshold       float64 `json:"high_fatigue_threshold"`
	MediumFatigueThreshold     float64 `json:"medium_fatigue_threshold"`
	LowFatigueThreshold        float64 `json:"low_fatigue_threshold"`
	VeryLowFatigueThreshold    float64 `json:"very_low_fatigue_threshold"`
	LowFireRateMajorThreshold  float64 `json:"low_fire_rate_major_threshold"`
	LowFireRateMinorThreshold  float64 `json:"low_fire_rate_minor_threshold"`
	HighFireRateThreshold      float64 `json:"high_fire_rate_threshold"`
	TargetFireRate             float64 `json:"target_fire_rate"`
	CooldownSteps              int     `json:"cooldown_steps"`
	CooldownFatigueReduction   float64 `json:"cooldown_fatigue_reduction"`
	NonCooldownFatigueReduction float64 `json:"non_cooldown_fatigue_reduction"`
	BaseFatigueGrowth          float64 `json:"base_fatigue_growth"`
	IntervalFactorFatigueGrowth float64 `json:"interval_factor_fatigue_growth"`
	FatigueAccelerationFactor  float64 `json:"fatigue_acceleration_factor"`
	BaseTension                float64 `json:"base_tension"`
	IntervalFactorTension      float64 `json:"interval_factor_tension"`
	MinIntervalMultiplier      float64 `json:"min_interval_multiplier"`
	MaxIntervalMultiplier      float64 `json:"max_interval_multiplier"`
	TensionHistorySize         int     `json:"tension_history_size"`
}

type ExplorationConfig struct {
	EpsilonGreedy      bool    `json:"epsilon_greedy"`
	SoftmaxTemperature float64 `json:"softmax_temperature"`
	NoisyNets          bool    `json:"noisy_nets"`
	ParameterSpaceNoise bool   `json:"parameter_space_noise"`
}

type LoggingConfig struct {
	LogInterval           int  `json:"log_interval"`
	SaveModelInterval     int  `json:"save_model_interval"`
	TensorboardLogging    bool `json:"tensorboard_logging"`
	MetricsExportInterval int  `json:"metrics_export_interval"`
}

var AppConfig *Config
var MechParams *MechanismParams
var RLConfig *RLParams

func Load() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("../config")
	viper.AddConfigPath("/etc/crossbow-simulation")

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		return fmt.Errorf("error unmarshaling config: %w", err)
	}

	if err := LoadMechanismParams(); err != nil {
		return fmt.Errorf("error loading mechanism params: %w", err)
	}

	if err := LoadRLParams(); err != nil {
		return fmt.Errorf("error loading RL params: %w", err)
	}

	log.Println("Configuration loaded successfully")
	return nil
}

func LoadMechanismParams() error {
	data, err := readJSONConfig("mechanism_params.json")
	if err != nil {
		return err
	}

	MechParams = &MechanismParams{}
	if err := json.Unmarshal(data, MechParams); err != nil {
		return fmt.Errorf("error unmarshaling mechanism params: %w", err)
	}

	log.Println("Mechanism parameters loaded successfully")
	return nil
}

func LoadRLParams() error {
	data, err := readJSONConfig("rl_params.json")
	if err != nil {
		return err
	}

	RLConfig = &RLParams{}
	if err := json.Unmarshal(data, RLConfig); err != nil {
		return fmt.Errorf("error unmarshaling RL params: %w", err)
	}

	log.Println("RL parameters loaded successfully")
	return nil
}

func readJSONConfig(filename string) ([]byte, error) {
	paths := []string{
		"./config/" + filename,
		"../config/" + filename,
		"/etc/crossbow-simulation/" + filename,
	}

	for _, path := range paths {
		if data, err := os.ReadFile(path); err == nil {
			return data, nil
		}
	}

	return nil, fmt.Errorf("config file %s not found in any search path", filename)
}

func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}

func (c *RedisConfig) GetAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
