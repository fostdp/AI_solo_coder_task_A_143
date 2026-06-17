-- ============================================================
-- 古代连弩动力学仿真系统 - 数据库初始化脚本
-- ============================================================

-- 1. 创建扩展
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 2. 创建连弩配置表
CREATE TABLE IF NOT EXISTS crossbows (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'idle',
    config JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. 创建传感器读数表
CREATE TABLE IF NOT EXISTS sensor_readings (
    time TIMESTAMPTZ NOT NULL,
    crossbow_id UUID NOT NULL REFERENCES crossbows(id) ON DELETE CASCADE,
    string_tension FLOAT NOT NULL,
    bow_arm_deformation FLOAT NOT NULL,
    magazine_position FLOAT NOT NULL,
    fire_rate FLOAT NOT NULL,
    arrow_velocity FLOAT NOT NULL,
    cam_angle FLOAT NOT NULL,
    string_fatigue FLOAT NOT NULL DEFAULT 0,
    temperature FLOAT NOT NULL DEFAULT 25.0
);

-- 4. 创建超表
SELECT create_hypertable('sensor_readings', 'time', if_not_exists => TRUE);

-- 5. 创建索引
CREATE INDEX IF NOT EXISTS idx_sensor_readings_crossbow_time ON sensor_readings (crossbow_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_readings_tension ON sensor_readings (time DESC, string_tension);
CREATE INDEX IF NOT EXISTS idx_sensor_readings_fire_rate ON sensor_readings (time DESC, fire_rate);

-- 6. 创建动力学状态表
CREATE TABLE IF NOT EXISTS dynamics_states (
    time TIMESTAMPTZ NOT NULL,
    crossbow_id UUID NOT NULL REFERENCES crossbows(id) ON DELETE CASCADE,
    bow_arm_angle FLOAT NOT NULL,
    bow_arm_angular_vel FLOAT NOT NULL,
    bow_arm_angular_acc FLOAT NOT NULL,
    string_displacement FLOAT NOT NULL,
    string_velocity FLOAT NOT NULL,
    cam_position FLOAT NOT NULL,
    pawl_engaged BOOLEAN NOT NULL,
    loading_complete BOOLEAN NOT NULL,
    arrow_loaded BOOLEAN NOT NULL,
    forces JSONB
);

SELECT create_hypertable('dynamics_states', 'time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_dynamics_states_crossbow_time ON dynamics_states (crossbow_id, time DESC);

-- 7. 创建箭矢弹道表
CREATE TABLE IF NOT EXISTS arrow_trajectories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    crossbow_id UUID NOT NULL REFERENCES crossbows(id) ON DELETE CASCADE,
    fire_time TIMESTAMPTZ NOT NULL,
    positions JSONB NOT NULL,
    initial_velocity FLOAT NOT NULL,
    flight_time FLOAT NOT NULL,
    impact_point JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_arrow_trajectories_crossbow_time ON arrow_trajectories (crossbow_id, fire_time DESC);

-- 8. 创建告警表
CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    crossbow_id UUID NOT NULL REFERENCES crossbows(id) ON DELETE CASCADE,
    type VARCHAR(100) NOT NULL,
    level VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    value FLOAT NOT NULL,
    threshold FLOAT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged BOOLEAN NOT NULL DEFAULT FALSE,
    acknowledged_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_alerts_crossbow_time ON alerts (crossbow_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_level_time ON alerts (level, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_acknowledged_time ON alerts (acknowledged, created_at DESC);

-- 9. 创建告警阈值表
CREATE TABLE IF NOT EXISTS alert_thresholds (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    crossbow_id UUID NOT NULL REFERENCES crossbows(id) ON DELETE CASCADE,
    string_tension_max FLOAT NOT NULL DEFAULT 1200,
    string_fatigue_warning FLOAT NOT NULL DEFAULT 0.7,
    fire_rate_min FLOAT NOT NULL DEFAULT 6,
    deformation_max FLOAT NOT NULL DEFAULT 20,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(crossbow_id)
);

-- 10. 创建强化学习训练记录表
CREATE TABLE IF NOT EXISTS rl_training_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    crossbow_id UUID NOT NULL REFERENCES crossbows(id) ON DELETE CASCADE,
    episode INTEGER NOT NULL,
    total_reward FLOAT NOT NULL,
    average_reward FLOAT NOT NULL,
    epsilon FLOAT NOT NULL,
    policy JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rl_training_crossbow_episode ON rl_training_records (crossbow_id, episode DESC);

-- 11. 创建强化学习结果表
CREATE TABLE IF NOT EXISTS rl_results (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    crossbow_id UUID NOT NULL REFERENCES crossbows(id) ON DELETE CASCADE,
    optimized_fire_rate FLOAT NOT NULL,
    optimized_loading_interval FLOAT NOT NULL,
    fatigue_reduction FLOAT NOT NULL,
    efficiency_improvement FLOAT NOT NULL,
    sustained_fire_duration FLOAT NOT NULL,
    training_episodes INTEGER NOT NULL,
    convergence_reward FLOAT NOT NULL,
    final_policy JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 12. 创建连续聚合视图 - 每分钟统计
CREATE MATERIALIZED VIEW IF NOT EXISTS sensor_readings_1m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', time) AS bucket,
    crossbow_id,
    AVG(string_tension) AS avg_string_tension,
    MAX(string_tension) AS max_string_tension,
    MIN(string_tension) AS min_string_tension,
    AVG(fire_rate) AS avg_fire_rate,
    AVG(bow_arm_deformation) AS avg_deformation,
    AVG(string_fatigue) AS avg_fatigue
FROM sensor_readings
GROUP BY bucket, crossbow_id
WITH NO DATA;

-- 13. 添加连续聚合策略
SELECT add_continuous_aggregate_policy('sensor_readings_1m',
    start_offset => INTERVAL '1 hour',
    end_offset => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute',
    if_not_exists => TRUE);

-- 14. 添加数据保留策略
SELECT add_retention_policy('sensor_readings', INTERVAL '1 year', if_not_exists => TRUE);
SELECT add_retention_policy('dynamics_states', INTERVAL '3 months', if_not_exists => TRUE);

-- 15. 插入初始数据 - 示例连弩配置
INSERT INTO crossbows (name, description, status, config) VALUES
('诸葛连弩-001', '三国时期诸葛连弩复原研究模型', 'idle', '{
    "bowArmLength": 0.85,
    "bowArmStiffness": 12500,
    "stringLength": 1.2,
    "stringTension": 450,
    "stringFatigueLimit": 10000,
    "arrowMass": 0.035,
    "magazineCapacity": 10,
    "camRadius": 0.04,
    "camLift": 0.08,
    "frictionCoefficient": 0.15,
    "gravity": 9.81
}') ON CONFLICT DO NOTHING;

-- 16. 插入初始告警阈值配置
INSERT INTO alert_thresholds (crossbow_id, string_tension_max, string_fatigue_warning, fire_rate_min, deformation_max)
SELECT id, 1200, 0.7, 6, 20
FROM crossbows
WHERE name = '诸葛连弩-001'
ON CONFLICT (crossbow_id) DO NOTHING;

-- ============================================================
-- 数据库初始化完成
-- ============================================================
