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

-- 3. 创建传感器读数表（高频时序数据）
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
SELECT create_hypertable('sensor_readings', 'time',
    if_not_exists => TRUE,
    chunk_time_interval => INTERVAL '1 day',
    associated_schema_name => '_timescaledb_internal'
);

-- 5. 创建索引
CREATE INDEX IF NOT EXISTS idx_sensor_readings_crossbow_time
    ON sensor_readings (crossbow_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_readings_tension
    ON sensor_readings (time DESC, string_tension);
CREATE INDEX IF NOT EXISTS idx_sensor_readings_fire_rate
    ON sensor_readings (time DESC, fire_rate);
CREATE INDEX IF NOT EXISTS idx_sensor_readings_fatigue
    ON sensor_readings (time DESC, string_fatigue);

-- 6. 创建动力学状态表（高采样率，压缩存储）
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

SELECT create_hypertable('dynamics_states', 'time',
    if_not_exists => TRUE,
    chunk_time_interval => INTERVAL '6 hours'
);
CREATE INDEX IF NOT EXISTS idx_dynamics_states_crossbow_time
    ON dynamics_states (crossbow_id, time DESC);

-- ============================================================
-- 7. 连续聚合视图 (Continuous Aggregates)
-- ============================================================

-- 7.1 每分钟聚合（短期监控用）
CREATE MATERIALIZED VIEW IF NOT EXISTS sensor_readings_1m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', time) AS bucket,
    crossbow_id,
    AVG(string_tension)    AS avg_string_tension,
    MAX(string_tension)    AS max_string_tension,
    MIN(string_tension)    AS min_string_tension,
    STDDEV(string_tension) AS std_string_tension,
    AVG(fire_rate)         AS avg_fire_rate,
    MAX(fire_rate)         AS max_fire_rate,
    MIN(fire_rate)         AS min_fire_rate,
    AVG(bow_arm_deformation) AS avg_deformation,
    MAX(bow_arm_deformation) AS max_deformation,
    AVG(string_fatigue)    AS avg_fatigue,
    MAX(string_fatigue)    AS max_fatigue,
    AVG(arrow_velocity)    AS avg_velocity,
    MAX(arrow_velocity)    AS max_velocity,
    AVG(temperature)       AS avg_temperature,
    MAX(temperature)       AS max_temperature,
    COUNT(*)               AS sample_count
FROM sensor_readings
GROUP BY bucket, crossbow_id
WITH NO DATA;

-- 7.2 每小时聚合（日报、历史趋势用）
CREATE MATERIALIZED VIEW IF NOT EXISTS sensor_readings_1h
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', time) AS bucket,
    crossbow_id,
    AVG(string_tension)    AS avg_string_tension,
    MAX(string_tension)    AS max_string_tension,
    MIN(string_tension)    AS min_string_tension,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY string_tension) AS p95_tension,
    AVG(fire_rate)         AS avg_fire_rate,
    MAX(fire_rate)         AS max_fire_rate,
    SUM(CASE WHEN fire_rate > 0 THEN 1 ELSE 0 END) AS shots_count,
    AVG(bow_arm_deformation) AS avg_deformation,
    MAX(bow_arm_deformation) AS max_deformation,
    MAX(string_fatigue)    AS max_fatigue,
    LAST(string_fatigue, time) AS final_fatigue,
    AVG(arrow_velocity)    AS avg_velocity,
    MAX(arrow_velocity)    AS max_velocity,
    AVG(temperature)       AS avg_temperature,
    COUNT(*)               AS sample_count
FROM sensor_readings
GROUP BY bucket, crossbow_id
WITH NO DATA;

-- 7.3 每日聚合（周报、月报、历史分析用）
CREATE MATERIALIZED VIEW IF NOT EXISTS sensor_readings_1d
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', time) AS bucket,
    crossbow_id,
    AVG(string_tension)    AS avg_string_tension,
    MAX(string_tension)    AS max_string_tension,
    MIN(string_tension)    AS min_string_tension,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY string_tension) AS p95_tension,
    PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY string_tension) AS p99_tension,
    AVG(fire_rate)         AS avg_fire_rate,
    MAX(fire_rate)         AS max_fire_rate,
    SUM(CASE WHEN fire_rate > 0 THEN 1 ELSE 0 END) AS total_shots,
    AVG(bow_arm_deformation) AS avg_deformation,
    MAX(bow_arm_deformation) AS max_deformation,
    MAX(string_fatigue)    AS max_fatigue,
    LAST(string_fatigue, time) AS final_fatigue,
    (LAST(string_fatigue, time) - FIRST(string_fatigue, time)) AS fatigue_delta,
    AVG(arrow_velocity)    AS avg_velocity,
    MAX(arrow_velocity)    AS max_velocity,
    MIN(arrow_velocity)    AS min_velocity,
    AVG(temperature)       AS avg_temperature,
    MAX(temperature)       AS max_temperature,
    COUNT(*)               AS sample_count
FROM sensor_readings
GROUP BY bucket, crossbow_id
WITH NO DATA;

-- ============================================================
-- 8. 连续聚合刷新策略
-- ============================================================

-- 1分钟聚合：每1分钟刷新，涵盖前1小时到前1分钟
SELECT add_continuous_aggregate_policy('sensor_readings_1m',
    start_offset => INTERVAL '1 hour',
    end_offset   => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute',
    if_not_exists => TRUE
);

-- 1小时聚合：每15分钟刷新，涵盖前1天到前1小时
SELECT add_continuous_aggregate_policy('sensor_readings_1h',
    start_offset => INTERVAL '1 day',
    end_offset   => INTERVAL '1 hour',
    schedule_interval => INTERVAL '15 minutes',
    if_not_exists => TRUE
);

-- 1天聚合：每小时刷新，涵盖前7天到前1天
SELECT add_continuous_aggregate_policy('sensor_readings_1d',
    start_offset => INTERVAL '7 days',
    end_offset   => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

-- ============================================================
-- 9. 数据保留策略 (Retention Policies)
-- ============================================================

-- 原始传感器读数：保留30天
SELECT add_retention_policy('sensor_readings',
    INTERVAL '30 days',
    if_not_exists => TRUE
);

-- 高频动力学状态：保留7天
SELECT add_retention_policy('dynamics_states',
    INTERVAL '7 days',
    if_not_exists => TRUE
);

-- 1分钟聚合：保留90天
SELECT add_retention_policy(
    format('%I.%I', caggschema, caggname)::REGCLASS,
    INTERVAL '90 days',
    if_not_exists => TRUE
)
FROM timescaledb_information.continuous_aggregates
WHERE view_name = 'sensor_readings_1m';

-- 1小时聚合：保留1年
SELECT add_retention_policy(
    format('%I.%I', caggschema, caggname)::REGCLASS,
    INTERVAL '1 year',
    if_not_exists => TRUE
)
FROM timescaledb_information.continuous_aggregates
WHERE view_name = 'sensor_readings_1h';

-- 1天聚合：永久保留（不限期）
-- 不添加保留策略 = 永久保留

-- ============================================================
-- 10. 压缩策略 (Compression Policies)
-- ============================================================

ALTER TABLE sensor_readings SET (
    timescaledb.compress,
    timescaledb.compress_orderby = 'time DESC',
    timescaledb.compress_segmentby = 'crossbow_id'
);

SELECT add_compression_policy('sensor_readings',
    INTERVAL '3 days',
    if_not_exists => TRUE
);

ALTER TABLE dynamics_states SET (
    timescaledb.compress,
    timescaledb.compress_orderby = 'time DESC',
    timescaledb.compress_segmentby = 'crossbow_id'
);

SELECT add_compression_policy('dynamics_states',
    INTERVAL '1 day',
    if_not_exists => TRUE
);

-- ============================================================
-- 11. 其他业务表
-- ============================================================

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
CREATE INDEX IF NOT EXISTS idx_arrow_trajectories_crossbow_time
    ON arrow_trajectories (crossbow_id, fire_time DESC);

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
CREATE INDEX IF NOT EXISTS idx_rl_training_crossbow_episode
    ON rl_training_records (crossbow_id, episode DESC);

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

-- ============================================================
-- 12. 初始数据
-- ============================================================

INSERT INTO crossbows (id, name, description, status, config) VALUES
('550e8400-e29b-41d4-a716-446655440000',
 '诸葛连弩-001',
 '三国时期诸葛连弩复原研究模型',
 'idle',
 '{
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
 }')
ON CONFLICT (id) DO NOTHING;

INSERT INTO alert_thresholds (crossbow_id, string_tension_max, string_fatigue_warning, fire_rate_min, deformation_max)
VALUES ('550e8400-e29b-41d4-a716-446655440000', 1200, 0.7, 6, 20)
ON CONFLICT (crossbow_id) DO NOTHING;

-- ============================================================
-- 13. 数据库统计信息
-- ============================================================

ANALYZE sensor_readings;
ANALYZE dynamics_states;
ANALYZE alerts;

-- ============================================================
-- 数据库初始化完成
-- ============================================================
