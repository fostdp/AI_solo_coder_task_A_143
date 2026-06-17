package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"crossbow-simulation/backend/config"
	"crossbow-simulation/backend/internal/api"
	"crossbow-simulation/backend/internal/coordinator"
	"crossbow-simulation/backend/internal/middleware"
	"crossbow-simulation/backend/internal/model"
	"crossbow-simulation/backend/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	if err := config.Load(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := repository.InitDB(config.AppConfig.Database.GetDSN(),
		config.AppConfig.Database.MaxOpenConns,
		config.AppConfig.Database.MaxIdleConns); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer repository.CloseDB()

	repo := repository.NewRepository(repository.GetDB())

	coord := coordinator.NewCoordinator(
		config.MechParams,
		config.RLConfig,
		repo,
	)
	coord.Start()
	defer coord.Stop()

	crossbowList, _, err := repo.ListCrossbows(1, 1)
	if err != nil {
		log.Printf("Warning: Failed to get crossbow: %v", err)
	}

	defaultConfig := model.DefaultCrossbowConfig()
	var crossbowID string

	if len(crossbowList) == 0 {
		log.Println("No crossbow found, creating default...")
		defaultCrossbow := &model.Crossbow{
			ID:          "550e8400-e29b-41d4-a716-446655440000",
			Name:        "诸葛连弩-001",
			Description: "三国时期诸葛连弩复原研究模型",
			Status:      "idle",
			Config:      defaultConfig,
		}
		id, err := repo.CreateCrossbow(defaultCrossbow)
		if err != nil {
			log.Printf("Error creating default crossbow: %v", err)
		} else {
			crossbowID = id
			log.Printf("Default crossbow created successfully, ID: %s", id)
		}
	} else {
		crossbowID = crossbowList[0].ID
		defaultConfig = crossbowList[0].Config
		log.Printf("Using existing crossbow: %s (ID: %s)", crossbowList[0].Name, crossbowID)
	}

	if err := coord.CreateCrossbowInstance(crossbowID); err != nil {
		log.Printf("Warning: Failed to create crossbow instance: %v", err)
	}

	ctrl := api.NewController(repo, coord)

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.Use(middleware.PrometheusMetrics())

	api.SetupRoutes(r, ctrl)

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	go func() {
		pprofAddr := "0.0.0.0:6060"
		log.Printf("pprof debug server starting on %s", pprofAddr)
		if err := http.ListenAndServe(pprofAddr, nil); err != nil && err != http.ErrServerClosed {
			log.Printf("pprof server error: %v", err)
		}
	}()

	addr := fmt.Sprintf("%s:%d", config.AppConfig.Server.Host, config.AppConfig.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  time.Duration(config.AppConfig.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(config.AppConfig.Server.WriteTimeout) * time.Second,
	}

	go func() {
		log.Printf("Server starting on %s", addr)
		log.Printf("Metrics endpoint: http://%s/metrics", addr)
		log.Printf("pprof endpoint: http://localhost:6060/debug/pprof/")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	coord.StopSimulation(crossbowID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited properly")
}
