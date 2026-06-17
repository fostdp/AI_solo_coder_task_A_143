package api

import (
	"net/http"
	"strconv"
	"time"

	"crossbow-simulation/backend/internal/alarm_ws"
	"crossbow-simulation/backend/internal/coordinator"
	"crossbow-simulation/backend/internal/model"
	"crossbow-simulation/backend/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Controller struct {
	repo     *repository.Repository
	coord    *coordinator.Coordinator
	upgrader websocket.Upgrader
}

func NewController(repo *repository.Repository, coord *coordinator.Coordinator) *Controller {
	return &Controller{
		repo:  repo,
		coord: coord,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (ctrl *Controller) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "OK",
		Data: map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"status":    "healthy",
			"system": map[string]interface{}{
				"dtu_receiver":   ctrl.coord.GetDTUStats(),
				"alarm_service":  ctrl.coord.GetAlarmStats(),
				"websocket_clients": ctrl.coord.GetClientCount(),
				"crossbow_instances": ctrl.coord.ListCrossbows(),
			},
		},
	})
}

func (ctrl *Controller) GetCrossbows(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))

	crossbows, total, err := ctrl.repo.ListCrossbows(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to get crossbows",
		})
		return
	}

	// 补充运行状态
	for _, cb := range crossbows {
		cb.Status = "idle"
		if ctrl.coord.IsSimulatorRunning(cb.ID) {
			cb.Status = "running"
		}
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"items":     crossbows,
			"total":     total,
			"page":      page,
			"pageSize":  pageSize,
		},
	})
}

func (ctrl *Controller) GetCrossbow(c *gin.Context) {
	id := c.Param("id")
	crossbow, err := ctrl.repo.GetCrossbowByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, model.APIResponse{
			Success: false,
			Message: "Crossbow not found",
		})
		return
	}

	// 补充运行状态
	crossbow.Status = "idle"
	if ctrl.coord.IsSimulatorRunning(id) {
		crossbow.Status = "running"
	}

	// 补充当前状态
	simState := ctrl.coord.GetSimulatorState(id)
	fatigueState := ctrl.coord.GetFatigueState(id)
	rlStatus := ctrl.coord.GetRLStatus(id)

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"crossbow": crossbow,
			"simulation": simState,
			"fatigue": fatigueState,
			"rl": rlStatus,
		},
	})
}

func (ctrl *Controller) CreateCrossbow(c *gin.Context) {
	var crossbow model.Crossbow
	if err := c.ShouldBindJSON(&crossbow); err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}

	crossbow.ID = uuid.New().String()
	crossbow.Status = "idle"
	crossbow.CreatedAt = time.Now()
	crossbow.UpdatedAt = time.Now()

	id, err := ctrl.repo.CreateCrossbow(&crossbow)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to create crossbow",
		})
		return
	}

	if err := ctrl.coord.CreateCrossbowInstance(id); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to create simulation instance",
		})
		return
	}

	crossbow.ID = id
	c.JSON(http.StatusCreated, model.APIResponse{
		Success: true,
		Data:    crossbow,
	})
}

func (ctrl *Controller) UpdateCrossbow(c *gin.Context) {
	id := c.Param("id")
	var crossbow model.Crossbow
	if err := c.ShouldBindJSON(&crossbow); err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}

	crossbow.ID = id
	crossbow.UpdatedAt = time.Now()

	if err := ctrl.repo.UpdateCrossbow(&crossbow); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to update crossbow",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    crossbow,
	})
}

func (ctrl *Controller) StartSimulation(c *gin.Context) {
	id := c.Param("id")
	var req model.StartSimulationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.SimulationSpeed = 1.0
		req.EnableRL = true
	}

	if err := ctrl.repo.UpdateCrossbowStatus(id, "running"); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to update crossbow status",
		})
		return
	}

	go ctrl.coord.StartSimulation(id, req.SimulationSpeed, req.EnableRL)

	sessionID := uuid.New().String()
	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "Simulation started",
		Data: map[string]interface{}{
			"sessionId": sessionID,
		},
	})
}

func (ctrl *Controller) StopSimulation(c *gin.Context) {
	id := c.Param("id")
	ctrl.coord.StopSimulation(id)

	if err := ctrl.repo.UpdateCrossbowStatus(id, "idle"); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to update crossbow status",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "Simulation stopped",
	})
}

func (ctrl *Controller) ResetSimulation(c *gin.Context) {
	id := c.Param("id")
	ctrl.coord.ResetSimulation(id)

	if err := ctrl.repo.UpdateCrossbowStatus(id, "idle"); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to update crossbow status",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "Simulation reset",
	})
}

func (ctrl *Controller) ReceiveSensorData(c *gin.Context) {
	var sensorData model.SensorData
	if err := c.ShouldBindJSON(&sensorData); err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "Invalid sensor data",
		})
		return
	}

	if sensorData.Timestamp.IsZero() {
		sensorData.Timestamp = time.Now()
	}

	if err := ctrl.coord.ReceiveSensorData(sensorData); err != nil {
		c.JSON(http.StatusServiceUnavailable, model.APIResponse{
			Success: false,
			Message: "Receiver is unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"recorded": true,
		},
	})
}

func (ctrl *Controller) QueryData(c *gin.Context) {
	var req model.DataQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "Invalid query request",
		})
		return
	}

	data, err := ctrl.repo.QuerySensorDataByTimeRange(
		req.CrossbowID,
		req.StartTime,
		req.EndTime,
		req.Metrics,
		req.Aggregation,
		req.Interval,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to query data",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    data,
	})
}

func (ctrl *Controller) GetAlerts(c *gin.Context) {
	crossbowID := c.Query("crossbowId")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	alerts := ctrl.coord.GetAlerts(crossbowID, limit)
	activeAlerts := ctrl.coord.GetActiveAlerts(crossbowID)

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"items":         alerts,
			"active_alerts": activeAlerts,
			"total":         len(alerts),
		},
	})
}

func (ctrl *Controller) AcknowledgeAlert(c *gin.Context) {
	id := c.Param("id")
	if ok := ctrl.coord.ResolveAlert(id); !ok {
		c.JSON(http.StatusNotFound, model.APIResponse{
			Success: false,
			Message: "Alert not found",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "Alert acknowledged",
	})
}

func (ctrl *Controller) StartRLTraining(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		MagazineCapacity int `json:"magazineCapacity"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.MagazineCapacity = 10
	}

	ctrl.coord.StartRLTraining(id, req.MagazineCapacity)

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "RL training started",
	})
}

func (ctrl *Controller) GetRLStatus(c *gin.Context) {
	id := c.Param("id")
	status := ctrl.coord.GetRLStatus(id)

	if status == nil {
		c.JSON(http.StatusNotFound, model.APIResponse{
			Success: false,
			Message: "RL status not available",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    status,
	})
}

func (ctrl *Controller) GetRLResult(c *gin.Context) {
	id := c.Param("id")
	result := ctrl.coord.GetOptimizedPolicy(id)

	if result == nil {
		c.JSON(http.StatusNotFound, model.APIResponse{
			Success: false,
			Message: "Optimized policy not available",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    result,
	})
}

func (ctrl *Controller) PauseRLTraining(c *gin.Context) {
	id := c.Param("id")
	ctrl.coord.PauseRLTraining(id)
	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "RL training paused",
	})
}

func (ctrl *Controller) ResumeRLTraining(c *gin.Context) {
	id := c.Param("id")
	ctrl.coord.ResumeRLTraining(id)
	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "RL training resumed",
	})
}

func (ctrl *Controller) WebSocketHandler(c *gin.Context) {
	crossbowID := c.Param("id")
	conn, err := ctrl.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &alarm_ws.Client{
		Conn:       conn,
		Send:       make(chan []byte, 256),
		CrossbowID: crossbowID,
	}

	ctrl.coord.RegisterWebSocketClient(client)

	go ctrl.coord.WritePump(client)
	go ctrl.coord.ReadPump(client)
}

func (ctrl *Controller) GetSystemStats(c *gin.Context) {
	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"dtu_receiver":      ctrl.coord.GetDTUStats(),
			"alarm_service":     ctrl.coord.GetAlarmStats(),
			"websocket_clients": ctrl.coord.GetClientCount(),
			"crossbow_instances": ctrl.coord.ListCrossbows(),
		},
	})
}
