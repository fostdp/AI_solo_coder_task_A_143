package api

import (
	"net/http"
	"strconv"
	"time"

	"crossbow-simulation/backend/internal/model"
	"crossbow-simulation/backend/internal/repository"
	"crossbow-simulation/backend/internal/simulation"
	"crossbow-simulation/backend/internal/rl"
	"crossbow-simulation/backend/internal/alert"
	"crossbow-simulation/backend/internal/websocket"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Controller struct {
	repo        *repository.Repository
	simService  *simulation.SimulationService
	rlService    *rl.RLService
	alertService *alert.AlertService
	wsHub        *websocket.Hub
	upgrader     websocket.Upgrader
}

func NewController(repo *repository.Repository, simService *simulation.SimulationService,
	rlService *rl.RLService, alertService *alert.AlertService, wsHub *websocket.Hub) *Controller {
	return &Controller{
		repo:        repo,
		simService:  simService,
		rlService:    rlService,
		alertService: alertService,
		wsHub:        wsHub,
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

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"items": crossbows,
			"total": total,
			"page":  page,
			"pageSize": pageSize,
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

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    crossbow,
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

	go ctrl.simService.Start(id, req.SimulationSpeed, req.EnableRL)

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
	ctrl.simService.Stop(id)

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
	ctrl.simService.Reset(id)

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

	if err := ctrl.repo.InsertSensorData(&sensorData); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to save sensor data",
		})
		return
	}

	ctrl.wsHub.BroadcastSensorData(sensorData.CrossbowID, &sensorData)
	ctrl.alertService.ProcessSensorData(sensorData.CrossbowID, &sensorData)

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
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	alerts, total, err := ctrl.alertService.GetAlerts(crossbowID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to get alerts",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"items": alerts,
			"total": total,
			"page":  page,
			"pageSize": pageSize,
		},
	})
}

func (ctrl *Controller) AcknowledgeAlert(c *gin.Context) {
	id := c.Param("id")
	if err := ctrl.alertService.AcknowledgeAlert(id); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to acknowledge alert",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "Alert acknowledged",
	})
}

func (ctrl *Controller) GetAlertThresholds(c *gin.Context) {
	crossbowID := c.Param("id")
	thresholds, err := ctrl.alertService.GetThresholds(crossbowID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to get thresholds",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    thresholds,
	})
}

func (ctrl *Controller) UpdateAlertThresholds(c *gin.Context) {
	var thresholds model.AlertThresholds
	if err := c.ShouldBindJSON(&thresholds); err != nil {
		c.JSON(http.StatusBadRequest, model.APIResponse{
			Success: false,
			Message: "Invalid thresholds data",
		})
		return
	}

	thresholds.UpdatedAt = time.Now()
	if err := ctrl.alertService.UpdateThresholds(&thresholds); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: "Failed to update thresholds",
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    thresholds,
	})
}

func (ctrl *Controller) StartRLTraining(c *gin.Context) {
	id := c.Param("id")
	ctrl.rlService.StartTraining(id, 1000)
	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "RL training started",
	})
}

func (ctrl *Controller) GetRLStatus(c *gin.Context) {
	id := c.Param("id")
	status := ctrl.rlService.GetStatus()
	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    status,
	})
}

func (ctrl *Controller) GetRLResult(c *gin.Context) {
	id := c.Param("id")
	result, err := ctrl.rlService.GetOptimizedPolicy()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Data:    result,
	})
}

func (ctrl *Controller) PauseRLTraining(c *gin.Context) {
	ctrl.rlService.PauseTraining()
	c.JSON(http.StatusOK, model.APIResponse{
		Success: true,
		Message: "RL training paused",
	})
}

func (ctrl *Controller) ResumeRLTraining(c *gin.Context) {
	ctrl.rlService.ResumeTraining()
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

	client := &websocket.Client{
		Conn:       conn,
		Send:       make(chan []byte, 256),
		CrossbowID: crossbowID,
	}

	ctrl.wsHub.Register <- client

	go client.WritePump(ctrl.wsHub)
	go client.ReadPump(ctrl.wsHub)
}
