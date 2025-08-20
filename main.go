package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

func main() {
	// Parse command line flags
	var healthCheck = flag.Bool("health-check", false, "Run health check and exit")
	flag.Parse()

	// Handle health check
	if *healthCheck {
		os.Exit(runHealthCheck())
	}

	// Initialize logger
	logger := logrus.New()
	
	// Configure log level
	logLevel := os.Getenv("LOG_LEVEL")
	switch logLevel {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "warn":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}
	
	logger.SetFormatter(&logrus.JSONFormatter{})

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		logger.WithError(err).Warn("Failed to load .env file")
	}

	// Validate required environment variables
	requiredEnvVars := []string{
		"TELEGRAM_BOT_TOKEN",
		"AUTH_CODE",
		"ROUTER_HOST",
		"ROUTER_USERNAME",
		"ROUTER_PASSWORD",
	}

	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			logger.Fatalf("Required environment variable %s is not set", envVar)
		}
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize SSH client
	sshClient, err := NewSSHClient(
		os.Getenv("ROUTER_HOST"),
		os.Getenv("ROUTER_USERNAME"),
		os.Getenv("ROUTER_PASSWORD"),
		logger,
	)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize SSH client")
	}

	// Initialize VPN manager
	vpnManager := NewVPNManager(sshClient, logger)

	// Initialize Telegram bot
	bot, err := NewTelegramBot(
		os.Getenv("TELEGRAM_BOT_TOKEN"),
		os.Getenv("AUTH_CODE"),
		vpnManager,
		logger,
	)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize Telegram bot")
	}

	// Start health check server
	healthServer := &http.Server{
		Addr:    ":8080",
		Handler: createHealthCheckHandler(bot, vpnManager, logger),
	}
	
	go func() {
		logger.Info("Starting health check server on :8080")
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithError(err).Error("Health check server failed")
		}
	}()

	// Start the bot
	go func() {
		if err := bot.Start(ctx); err != nil {
			logger.WithError(err).Error("Bot stopped with error")
			cancel()
		}
	}()

	logger.Info("VPN Commander bot started successfully")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.WithField("signal", sig).Info("Received shutdown signal")
	case <-ctx.Done():
		logger.Info("Context cancelled")
	}

	// Graceful shutdown
	logger.Info("Shutting down gracefully...")
	cancel()

	// Shutdown health server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Error("Failed to shutdown health server")
	}

	// Give components time to shut down
	time.Sleep(2 * time.Second)
	logger.Info("Shutdown complete")
}

// runHealthCheck performs a simple health check
func runHealthCheck() int {
	// Simple health check - just verify the process can start
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	// Check if required env vars are set
	requiredVars := []string{
		"TELEGRAM_BOT_TOKEN",
		"AUTH_CODE", 
		"ROUTER_HOST",
		"ROUTER_USERNAME",
		"ROUTER_PASSWORD",
	}
	
	for _, envVar := range requiredVars {
		if os.Getenv(envVar) == "" {
			logger.Errorf("Health check failed: %s not set", envVar)
			return 1
		}
	}
	
	logger.Info("Health check passed")
	return 0
}

// createHealthCheckHandler creates HTTP handlers for health checks
func createHealthCheckHandler(bot *TelegramBot, vpnManager *VPNManager, logger *logrus.Logger) http.Handler {
	mux := http.NewServeMux()
	
	// Liveness probe
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	// Readiness probe
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// Check if bot is ready
		if bot == nil {
			http.Error(w, "Bot not initialized", http.StatusServiceUnavailable)
			return
		}
		
		// Check if VPN manager is ready
		if vpnManager == nil {
			http.Error(w, "VPN manager not initialized", http.StatusServiceUnavailable)
			return
		}
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	})
	
	// Status endpoint
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		statusData := map[string]interface{}{
			"status": "running",
			"bot": map[string]interface{}{
				"username": bot.GetBotInfo().UserName,
			},
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(statusData)
	})
	
	return mux
}