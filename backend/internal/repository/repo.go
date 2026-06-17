package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"crossbow-simulation/backend/internal/model"
)

type Repository struct {
	db *Database
}

func NewRepository(db *Database) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateCrossbow(crossbow *model.Crossbow) (string, error) {
	if crossbow == nil {
		return "", errors.New("crossbow is nil")
	}
	if crossbow.Name == "" {
		return "", errors.New("crossbow name is required")
	}

	configJSON, err := json.Marshal(crossbow.Config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal crossbow config: %w", err)
	}

	now := time.Now()
	if crossbow.CreatedAt.IsZero() {
		crossbow.CreatedAt = now
	}
	crossbow.UpdatedAt = now
	if crossbow.Status == "" {
		crossbow.Status = "idle"
	}

	var id string
	query := `
		INSERT INTO crossbows (id, name, description, status, config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`

	err = r.db.Pool.QueryRow(context.Background(), query,
		crossbow.ID,
		crossbow.Name,
		crossbow.Description,
		crossbow.Status,
		configJSON,
		crossbow.CreatedAt,
		crossbow.UpdatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("failed to create crossbow: %w", err)
	}

	return id, nil
}

func (r *Repository) GetCrossbowByID(id string) (*model.Crossbow, error) {
	if id == "" {
		return nil, errors.New("crossbow id is required")
	}

	query := `
		SELECT id, name, description, status, config, created_at, updated_at
		FROM crossbows
		WHERE id = $1
	`

	var crossbow model.Crossbow
	var configJSON []byte

	err := r.db.Pool.QueryRow(context.Background(), query, id).Scan(
		&crossbow.ID,
		&crossbow.Name,
		&crossbow.Description,
		&crossbow.Status,
		&configJSON,
		&crossbow.CreatedAt,
		&crossbow.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("crossbow not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get crossbow: %w", err)
	}

	if err := json.Unmarshal(configJSON, &crossbow.Config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal crossbow config: %w", err)
	}

	return &crossbow, nil
}

func (r *Repository) ListCrossbows(page, pageSize int) ([]model.Crossbow, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	countQuery := `SELECT COUNT(*) FROM crossbows`
	var total int64
	err := r.db.Pool.QueryRow(context.Background(), countQuery).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count crossbows: %w", err)
	}

	query := `
		SELECT id, name, description, status, config, created_at, updated_at
		FROM crossbows
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Pool.Query(context.Background(), query, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list crossbows: %w", err)
	}
	defer rows.Close()

	var crossbows []model.Crossbow
	for rows.Next() {
		var crossbow model.Crossbow
		var configJSON []byte

		err := rows.Scan(
			&crossbow.ID,
			&crossbow.Name,
			&crossbow.Description,
			&crossbow.Status,
			&configJSON,
			&crossbow.CreatedAt,
			&crossbow.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan crossbow row: %w", err)
		}

		if err := json.Unmarshal(configJSON, &crossbow.Config); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal crossbow config: %w", err)
		}

		crossbows = append(crossbows, crossbow)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration error: %w", err)
	}

	return crossbows, total, nil
}

func (r *Repository) UpdateCrossbow(crossbow *model.Crossbow) error {
	if crossbow == nil {
		return errors.New("crossbow is nil")
	}
	if crossbow.ID == "" {
		return errors.New("crossbow id is required")
	}

	configJSON, err := json.Marshal(crossbow.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal crossbow config: %w", err)
	}

	crossbow.UpdatedAt = time.Now()

	query := `
		UPDATE crossbows
		SET name = $1, description = $2, status = $3, config = $4, updated_at = $5
		WHERE id = $6
	`

	result, err := r.db.Pool.Exec(context.Background(), query,
		crossbow.Name,
		crossbow.Description,
		crossbow.Status,
		configJSON,
		crossbow.UpdatedAt,
		crossbow.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update crossbow: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("crossbow not found with id: %s", crossbow.ID)
	}

	return nil
}

func (r *Repository) UpdateCrossbowStatus(id, status string) error {
	if id == "" {
		return errors.New("crossbow id is required")
	}
	if status == "" {
		return errors.New("status is required")
	}

	query := `
		UPDATE crossbows
		SET status = $1, updated_at = $2
		WHERE id = $3
	`

	result, err := r.db.Pool.Exec(context.Background(), query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update crossbow status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("crossbow not found with id: %s", id)
	}

	return nil
}

func (r *Repository) DeleteCrossbow(id string) error {
	if id == "" {
		return errors.New("crossbow id is required")
	}

	query := `DELETE FROM crossbows WHERE id = $1`

	result, err := r.db.Pool.Exec(context.Background(), query, id)
	if err != nil {
		return fmt.Errorf("failed to delete crossbow: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("crossbow not found with id: %s", id)
	}

	return nil
}

func (r *Repository) InsertSensorData(data *model.SensorData) error {
	if data == nil {
		return errors.New("sensor data is nil")
	}
	if data.CrossbowID == "" {
		return errors.New("crossbow id is required")
	}

	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	query := `
		INSERT INTO sensor_readings (
			time, crossbow_id, string_tension, bow_arm_deformation,
			magazine_position, fire_rate, arrow_velocity, cam_angle,
			string_fatigue, temperature
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.Pool.Exec(context.Background(), query,
		data.Timestamp,
		data.CrossbowID,
		data.StringTension,
		data.BowArmDeformation,
		data.MagazinePosition,
		data.FireRate,
		data.ArrowVelocity,
		data.CamAngle,
		data.StringFatigue,
		data.Temperature,
	)
	if err != nil {
		return fmt.Errorf("failed to insert sensor data: %w", err)
	}

	return nil
}

func (r *Repository) GetLatestSensorData(crossbowID string) (*model.SensorData, error) {
	if crossbowID == "" {
		return nil, errors.New("crossbow id is required")
	}

	query := `
		SELECT time, crossbow_id, string_tension, bow_arm_deformation,
			   magazine_position, fire_rate, arrow_velocity, cam_angle,
			   string_fatigue, temperature
		FROM sensor_readings
		WHERE crossbow_id = $1
		ORDER BY time DESC
		LIMIT 1
	`

	var data model.SensorData
	err := r.db.Pool.QueryRow(context.Background(), query, crossbowID).Scan(
		&data.Timestamp,
		&data.CrossbowID,
		&data.StringTension,
		&data.BowArmDeformation,
		&data.MagazinePosition,
		&data.FireRate,
		&data.ArrowVelocity,
		&data.CamAngle,
		&data.StringFatigue,
		&data.Temperature,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &model.SensorData{
				CrossbowID:        crossbowID,
				Timestamp:         time.Now(),
				StringTension:     450,
				BowArmDeformation: 5,
				MagazinePosition:  0,
				FireRate:          0,
				ArrowVelocity:     0,
				CamAngle:          0,
				StringFatigue:     0,
				Temperature:       25,
			}, nil
		}
		return nil, fmt.Errorf("failed to get latest sensor data: %w", err)
	}

	return &data, nil
}

func (r *Repository) QuerySensorDataByTimeRange(crossbowID, startTime, endTime string, metrics []string, aggregation, interval string) ([]map[string]interface{}, error) {
	if crossbowID == "" {
		return nil, errors.New("crossbow id is required")
	}

	if len(metrics) == 0 {
		metrics = []string{"string_tension", "bow_arm_deformation", "magazine_position", "fire_rate"}
	}

	metricMap := map[string]string{
		"stringTension":     "string_tension",
		"bowArmDeformation": "bow_arm_deformation",
		"magazinePosition":  "magazine_position",
		"fireRate":          "fire_rate",
		"arrowVelocity":     "arrow_velocity",
		"camAngle":          "cam_angle",
		"stringFatigue":     "string_fatigue",
		"temperature":       "temperature",
	}

	aggMap := map[string]string{
		"avg":   "AVG",
		"max":   "MAX",
		"min":   "MIN",
		"sum":   "SUM",
		"count": "COUNT",
	}

	aggFunc := "AVG"
	if v, ok := aggMap[aggregation]; ok {
		aggFunc = v
	}

	selectCols := "time_bucket($3, time) AS bucket"
	for _, m := range metrics {
		if col, ok := metricMap[m]; ok {
			selectCols += fmt.Sprintf(", %s(%s) AS %s", aggFunc, col, m)
		}
	}

	if interval == "" {
		interval = "1 minute"
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM sensor_readings
		WHERE crossbow_id = $1 AND time >= $2 AND time <= $4
		GROUP BY bucket
		ORDER BY bucket DESC
		LIMIT 1000
	`, selectCols)

	rows, err := r.db.Pool.Query(context.Background(), query, crossbowID, startTime, interval, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query sensor data: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	fieldDescriptions := rows.FieldDescriptions()

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("failed to get row values: %w", err)
		}

		row := make(map[string]interface{})
		for i, fd := range fieldDescriptions {
			row[string(fd.Name)] = values[i]
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return results, nil
}

func (r *Repository) InsertDynamicsState(state *model.DynamicsState) error {
	if state == nil {
		return errors.New("dynamics state is nil")
	}

	if state.Timestamp.IsZero() {
		state.Timestamp = time.Now()
	}

	query := `
		INSERT INTO dynamics_states (
			time, crossbow_id, bow_arm_angle, bow_arm_angular_vel, bow_arm_angular_acc,
			string_displacement, string_velocity, cam_position, pawl_engaged,
			loading_complete, arrow_loaded, forces
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err := r.db.Pool.Exec(context.Background(), query,
		state.Timestamp,
		"",
		state.BowArmAngle,
		state.BowArmAngularVel,
		state.BowArmAngularAcc,
		state.StringDisplacement,
		state.StringVelocity,
		state.CamPosition,
		state.PawlEngaged,
		state.LoadingComplete,
		state.ArrowLoaded,
		state.Forces,
	)
	if err != nil {
		return fmt.Errorf("failed to insert dynamics state: %w", err)
	}

	return nil
}

func (r *Repository) InsertTrajectory(trajectory *model.ArrowTrajectory) (string, error) {
	if trajectory == nil {
		return "", errors.New("trajectory is nil")
	}

	positionsJSON, err := json.Marshal(trajectory.Positions)
	if err != nil {
		return "", fmt.Errorf("failed to marshal positions: %w", err)
	}

	impactJSON, err := json.Marshal(trajectory.ImpactPoint)
	if err != nil {
		return "", fmt.Errorf("failed to marshal impact point: %w", err)
	}

	if trajectory.CreatedAt.IsZero() {
		trajectory.CreatedAt = time.Now()
	}

	var id string
	query := `
		INSERT INTO arrow_trajectories (
			id, crossbow_id, fire_time, positions, initial_velocity,
			flight_time, impact_point, created_at
		) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`

	err = r.db.Pool.QueryRow(context.Background(), query,
		trajectory.CrossbowID,
		trajectory.FireTime,
		positionsJSON,
		trajectory.InitialVelocity,
		trajectory.FlightTime,
		impactJSON,
		trajectory.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("failed to insert trajectory: %w", err)
	}

	return id, nil
}

func (r *Repository) CreateAlert(alert *model.Alert) (string, error) {
	if alert == nil {
		return "", errors.New("alert is nil")
	}
	if alert.CrossbowID == "" {
		return "", errors.New("crossbow id is required")
	}

	if alert.CreatedAt.IsZero() {
		alert.CreatedAt = time.Now()
	}

	var id string
	query := `
		INSERT INTO alerts (
			id, crossbow_id, type, level, message, value, threshold,
			created_at, acknowledged
		) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`

	err := r.db.Pool.QueryRow(context.Background(), query,
		alert.CrossbowID,
		alert.Type,
		alert.Level,
		alert.Message,
		alert.Value,
		alert.Threshold,
		alert.CreatedAt,
		alert.Acknowledged,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("failed to create alert: %w", err)
	}

	return id, nil
}

func (r *Repository) ListAlerts(filters map[string]interface{}, page, pageSize int) ([]model.Alert, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1

	if v, ok := filters["crossbow_id"].(string); ok && v != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("crossbow_id = $%d", argIndex))
		args = append(args, v)
		argIndex++
	}

	if v, ok := filters["acknowledged"].(bool); ok {
		whereClauses = append(whereClauses, fmt.Sprintf("acknowledged = $%d", argIndex))
		args = append(args, v)
		argIndex++
	}

	if v, ok := filters["level"].(string); ok && v != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("level = $%d", argIndex))
		args = append(args, v)
		argIndex++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM alerts WHERE %s`, whereSQL)
	var total int64
	err := r.db.Pool.QueryRow(context.Background(), countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count alerts: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, crossbow_id, type, level, message, value, threshold,
			   created_at, acknowledged, acknowledged_at
		FROM alerts
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, argIndex, argIndex+1)

	args = append(args, pageSize, offset)

	rows, err := r.db.Pool.Query(context.Background(), query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []model.Alert
	for rows.Next() {
		var alert model.Alert
		var ackAt *time.Time

		err := rows.Scan(
			&alert.ID,
			&alert.CrossbowID,
			&alert.Type,
			&alert.Level,
			&alert.Message,
			&alert.Value,
			&alert.Threshold,
			&alert.CreatedAt,
			&alert.Acknowledged,
			&ackAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan alert row: %w", err)
		}

		if ackAt != nil {
			alert.AcknowledgedAt = *ackAt
		}

		alerts = append(alerts, alert)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration error: %w", err)
	}

	return alerts, total, nil
}

func (r *Repository) AcknowledgeAlert(id string) error {
	if id == "" {
		return errors.New("alert id is required")
	}

	query := `
		UPDATE alerts
		SET acknowledged = true, acknowledged_at = $1
		WHERE id = $2
	`

	result, err := r.db.Pool.Exec(context.Background(), query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to acknowledge alert: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("alert not found with id: %s", id)
	}

	return nil
}

func (r *Repository) GetThresholdsByCrossbowID(crossbowID string) (*model.AlertThresholds, error) {
	if crossbowID == "" {
		return nil, errors.New("crossbow id is required")
	}

	query := `
		SELECT id, crossbow_id, string_tension_max, string_fatigue_warning,
			   fire_rate_min, deformation_max, created_at, updated_at
		FROM alert_thresholds
		WHERE crossbow_id = $1
	`

	var thresholds model.AlertThresholds
	err := r.db.Pool.QueryRow(context.Background(), query, crossbowID).Scan(
		&thresholds.ID,
		&thresholds.CrossbowID,
		&thresholds.StringTensionMax,
		&thresholds.StringFatigueWarning,
		&thresholds.FireRateMin,
		&thresholds.DeformationMax,
		&thresholds.CreatedAt,
		&thresholds.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &model.AlertThresholds{
				CrossbowID:          crossbowID,
				StringTensionMax:    1200,
				StringFatigueWarning: 0.7,
				FireRateMin:         6,
				DeformationMax:      20,
			}, nil
		}
		return nil, fmt.Errorf("failed to get thresholds: %w", err)
	}

	return &thresholds, nil
}

func (r *Repository) UpdateThresholds(thresholds *model.AlertThresholds) error {
	if thresholds == nil {
		return errors.New("thresholds is nil")
	}
	if thresholds.CrossbowID == "" {
		return errors.New("crossbow id is required")
	}

	thresholds.UpdatedAt = time.Now()

	query := `
		INSERT INTO alert_thresholds (
			id, crossbow_id, string_tension_max, string_fatigue_warning,
			fire_rate_min, deformation_max, created_at, updated_at
		) VALUES (
			COALESCE((SELECT id FROM alert_thresholds WHERE crossbow_id = $1), gen_random_uuid()),
			$1, $2, $3, $4, $5,
			COALESCE((SELECT created_at FROM alert_thresholds WHERE crossbow_id = $1), NOW()),
			$6
		)
		ON CONFLICT (crossbow_id) DO UPDATE SET
			string_tension_max = EXCLUDED.string_tension_max,
			string_fatigue_warning = EXCLUDED.string_fatigue_warning,
			fire_rate_min = EXCLUDED.fire_rate_min,
			deformation_max = EXCLUDED.deformation_max,
			updated_at = EXCLUDED.updated_at
	`

	_, err := r.db.Pool.Exec(context.Background(), query,
		thresholds.CrossbowID,
		thresholds.StringTensionMax,
		thresholds.StringFatigueWarning,
		thresholds.FireRateMin,
		thresholds.DeformationMax,
		thresholds.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update thresholds: %w", err)
	}

	return nil
}

func (r *Repository) CreateTrainingRecord(record *model.RLTrainingRecord) error {
	if record == nil {
		return errors.New("training record is nil")
	}
	if record.CrossbowID == "" {
		return errors.New("crossbow id is required")
	}

	policyJSON, err := json.Marshal(record.Policy)
	if err != nil {
		return fmt.Errorf("failed to marshal policy: %w", err)
	}

	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO rl_training_records (
			id, crossbow_id, episode, total_reward, average_reward,
			epsilon, policy, created_at
		) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7)
	`

	_, err = r.db.Pool.Exec(context.Background(), query,
		record.CrossbowID,
		record.Episode,
		record.TotalReward,
		record.AverageReward,
		record.Epsilon,
		policyJSON,
		record.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create training record: %w", err)
	}

	return nil
}

func (r *Repository) CreateResult(result *model.RLResult) error {
	if result == nil {
		return errors.New("result is nil")
	}
	if result.CrossbowID == "" {
		return errors.New("crossbow id is required")
	}

	policyJSON, err := json.Marshal(result.FinalPolicy)
	if err != nil {
		return fmt.Errorf("failed to marshal final policy: %w", err)
	}

	if result.CreatedAt.IsZero() {
		result.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO rl_results (
			id, crossbow_id, optimized_fire_rate, optimized_loading_interval,
			fatigue_reduction, efficiency_improvement, sustained_fire_duration,
			training_episodes, convergence_reward, final_policy, created_at
		) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err = r.db.Pool.Exec(context.Background(), query,
		result.CrossbowID,
		result.OptimizedFireRate,
		result.OptimizedLoadingInterval,
		result.FatigueReduction,
		result.EfficiencyImprovement,
		result.SustainedFireDuration,
		result.TrainingEpisodes,
		result.ConvergenceReward,
		policyJSON,
		result.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create result: %w", err)
	}

	return nil
}

func (r *Repository) GetLatestResult(crossbowID string) (*model.RLResult, error) {
	if crossbowID == "" {
		return nil, errors.New("crossbow id is required")
	}

	query := `
		SELECT id, crossbow_id, optimized_fire_rate, optimized_loading_interval,
			   fatigue_reduction, efficiency_improvement, sustained_fire_duration,
			   training_episodes, convergence_reward, final_policy, created_at
		FROM rl_results
		WHERE crossbow_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var result model.RLResult
	var policyJSON []byte

	err := r.db.Pool.QueryRow(context.Background(), query, crossbowID).Scan(
		&result.ID,
		&result.CrossbowID,
		&result.OptimizedFireRate,
		&result.OptimizedLoadingInterval,
		&result.FatigueReduction,
		&result.EfficiencyImprovement,
		&result.SustainedFireDuration,
		&result.TrainingEpisodes,
		&result.ConvergenceReward,
		&policyJSON,
		&result.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest result: %w", err)
	}

	if err := json.Unmarshal(policyJSON, &result.FinalPolicy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal final policy: %w", err)
	}

	return &result, nil
}
