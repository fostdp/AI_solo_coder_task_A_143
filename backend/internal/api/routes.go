package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine, ctrl *Controller) {
	r.Use(CORS())

	api := r.Group("/api/v1")
	{
		api.GET("/health", ctrl.HealthCheck)
		api.GET("/system-stats", ctrl.GetSystemStats)

		crossbows := api.Group("/crossbows")
		{
			crossbows.GET("", ctrl.GetCrossbows)
			crossbows.GET("/:id", ctrl.GetCrossbow)
			crossbows.POST("", ctrl.CreateCrossbow)
			crossbows.PUT("/:id", ctrl.UpdateCrossbow)
			crossbows.POST("/:id/start", ctrl.StartSimulation)
			crossbows.POST("/:id/stop", ctrl.StopSimulation)
			crossbows.POST("/:id/reset", ctrl.ResetSimulation)
		}

		sensor := api.Group("/sensor")
		{
			sensor.POST("/data", ctrl.ReceiveSensorData)
		}

		data := api.Group("/data")
		{
			data.POST("/query", ctrl.QueryData)
		}

		alerts := api.Group("/alerts")
		{
			alerts.GET("", ctrl.GetAlerts)
			alerts.POST("/:id/ack", ctrl.AcknowledgeAlert)
		}

		rl := api.Group("/rl")
		{
			rl.POST("/train/:id", ctrl.StartRLTraining)
			rl.GET("/status/:id", ctrl.GetRLStatus)
			rl.GET("/result/:id", ctrl.GetRLResult)
			rl.POST("/pause/:id", ctrl.PauseRLTraining)
			rl.POST("/resume/:id", ctrl.ResumeRLTraining)
		}
	}

	r.GET("/ws/crossbow/:id", ctrl.WebSocketHandler)

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "API endpoint not found"})
		}
	})
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
