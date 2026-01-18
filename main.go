package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"file-sync/config"
	"file-sync/db"
	"file-sync/logger"
	"file-sync/p2p"
	"file-sync/server"
	"file-sync/sync"
	"file-sync/ui"
)

func main() {
	logger.Info("=== File Sync Server Starting ===")

	// Check for demo mode and UI mode
	demoMode := false
	uiMode := false
	args := os.Args[1:]
	for _, arg := range args {
		switch arg {
		case "--demo":
			demoMode = true
		case "--ui":
			uiMode = true
		}
	}

	// Load configuration
	configPath := "config.yaml"
	if len(args) > 0 {
		configPath = args[0]
	}

	logger.Info("Loading configuration from: %s", configPath)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	if err := logger.InitDefaultLogger(cfg.Logging.File, uiMode); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	logger.Info("Configuration loaded successfully")
	logger.Info("Server Name: %s", cfg.Server.Name)
	logger.Info("Server Address: %s", cfg.GetServerAddr())
	logger.Info("Web Interface: %s", cfg.GetServerAddr())
	logger.Info("Sync Directory: %s", cfg.Server.SyncDir)
	logger.Info("Database: %s", cfg.Database.Path)
	logger.Info("Peers: %d", len(cfg.Peers))
	if demoMode {
		logger.Info("Running in DEMO mode (no network)")
	}
	if uiMode {
		logger.Info("Running in UI mode")
	}

	logger.Info("Starting File Sync Server: %s", cfg.Server.Name)

	// Initialize database
	database, err := db.NewDatabase(cfg.Database.Path)
	if err != nil {
		logger.Error("Failed to initialize database: %v", err)
		os.Exit(1)
	}
	defer database.Close()

	// Initialize P2P network
	p2pNetwork := p2p.NewP2PNetwork(cfg)
	p2pNetwork.SetDemoMode(demoMode)
	if err := p2pNetwork.Start(); err != nil {
		logger.Error("Failed to start P2P network: %v", err)
		os.Exit(1)
	}

	// Initialize UI manager
	var uiManager *ui.UIManager
	if uiMode {
		uiManager = ui.NewUIManager()
		logger.Info("UI mode enabled")
	}

	// Initialize file sync service
	syncService := sync.NewFileSync(cfg, database, p2pNetwork)
	if uiManager != nil {
		syncService.SetUIManager(uiManager)
	}
	if err := syncService.Start(); err != nil {
		logger.Error("Failed to start sync service: %v", err)
		os.Exit(1)
	}

	// Initialize web server
	webServer := server.NewServer(cfg, database, p2pNetwork, syncService)

	// Set UI manager for P2P network (for server status updates)
	if uiMode {
		p2pNetwork.SetUIManager(uiManager)
	}

	// Handle graceful shutdown
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		logger.Info("Shutting down gracefully...")
		if uiManager != nil {
			uiManager.Stop()
		}
		database.Close()
		os.Exit(0)
	}()

	// Start appropriate mode
	if uiMode {
		logger.Info("Starting UI mode")

		// Start background services
		go func() {
			// Start web server in background
			if !demoMode {
				if err := webServer.Start(); err != nil {
					logger.Error("Failed to start web server: %v", err)
				}
			}
		}()

		// Initialize UI with server data
		for name, peer := range cfg.Peers {
			uiManager.UpdateServerStatus(name, ui.ServerStatus{
				Name:   name,
				Host:   peer.Host,
				Port:   peer.Port,
				Status: "unknown",
			})
		}

		// Start UI (blocking call)
		logger.Info("Starting UI...")
		if err := uiManager.Start(); err != nil {
			log.Fatalf("Failed to start UI: %v", err)
		}

		// UI stopped, cleanup will be done by defer
		logger.Info("UI stopped, main function returning...")

	} else {
		// Start web server (skip in demo mode)
		if !demoMode {
			if err := webServer.Start(); err != nil {
				logger.Error("Failed to start web server: %v", err)
				os.Exit(1)
			}
		} else {
			logger.Info("Demo mode: skipping web server startup")
			logger.Info("Demo mode: running sync service only")

			// In demo mode, run a simple loop to demonstrate sync
			go func() {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()

				logger.Info("Starting demo sync loop (press Ctrl+C to exit)")

				for range ticker.C {
					logger.Info("Performing sync at %s", time.Now().Format("15:04:05"))
					if err := syncService.Start(); err != nil {
						logger.Error("Sync error: %v", err)
					}

					files, err := syncService.GetLocalFiles()
					if err != nil {
						logger.Error("Error getting local files: %v", err)
					} else {
						logger.Info("Local files: %d", len(files))
						for _, file := range files {
							logger.Debug("File: %s (size: %d)", file.Path, file.Size)
						}
					}
				}
			}()

			// Keep running in demo mode
			select {}
		}
	}
}
