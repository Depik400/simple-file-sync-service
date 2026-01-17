package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"file-sync/config"
	"file-sync/db"
	"file-sync/p2p"
	"file-sync/server"
	"file-sync/sync"
)

func main() {
	fmt.Printf("=== File Sync Server Starting ===\n")

	// Check for demo mode
	demoMode := false
	args := os.Args[1:]
	for i, arg := range args {
		if arg == "--demo" {
			demoMode = true
			// Remove --demo from args
			args = append(args[:i], args[i+1:]...)
			break
		}
	}

	// Load configuration
	configPath := "config.yaml"
	if len(args) > 0 {
		configPath = args[0]
	}

	fmt.Printf("[MAIN] Loading configuration from: %s\n", configPath)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("[MAIN] Configuration loaded successfully\n")
	fmt.Printf("[MAIN] Server Name: %s\n", cfg.Server.Name)
	fmt.Printf("[MAIN] Server Address: %s\n", cfg.GetServerAddr())
	fmt.Printf("[MAIN] Web Interface: %s\n", cfg.GetServerAddr())
	fmt.Printf("[MAIN] Sync Directory: %s\n", cfg.Server.SyncDir)
	fmt.Printf("[MAIN] Database: %s\n", cfg.Database.Path)
	fmt.Printf("[MAIN] Peers: %d\n", len(cfg.Peers))
	if demoMode {
		fmt.Printf("[MAIN] Running in DEMO mode (no network)\n")
	}

	fmt.Printf("[MAIN] Starting File Sync Server: %s\n", cfg.Server.Name)

	// Initialize database
	database, err := db.NewDatabase(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Initialize P2P network
	p2pNetwork := p2p.NewP2PNetwork(cfg)
	p2pNetwork.SetDemoMode(demoMode)
	if err := p2pNetwork.Start(); err != nil {
		log.Fatalf("Failed to start P2P network: %v", err)
	}

	// Initialize file sync service
	syncService := sync.NewFileSync(cfg, database, p2pNetwork)
	if err := syncService.Start(); err != nil {
		log.Fatalf("Failed to start sync service: %v", err)
	}

	// Initialize web server
	webServer := server.NewServer(cfg, database, p2pNetwork, syncService)

	// Handle graceful shutdown
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		fmt.Println("\nShutting down gracefully...")
		database.Close()
		os.Exit(0)
	}()

	// Start web server (skip in demo mode)
	if !demoMode {
		if err := webServer.Start(); err != nil {
			log.Fatalf("Failed to start web server: %v", err)
		}
	} else {
		fmt.Printf("[MAIN] Demo mode: skipping web server startup\n")
		fmt.Printf("[MAIN] Demo mode: running sync service only\n")

		// In demo mode, run a simple loop to demonstrate sync
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			fmt.Printf("[DEMO] Starting demo sync loop (press Ctrl+C to exit)\n")

			for range ticker.C {
				fmt.Printf("[DEMO] Performing sync at %s\n", time.Now().Format("15:04:05"))
				if err := syncService.Start(); err != nil {
					fmt.Printf("[DEMO] Sync error: %v\n", err)
				}

				files, err := syncService.GetLocalFiles()
				if err != nil {
					fmt.Printf("[DEMO] Error getting local files: %v\n", err)
				} else {
					fmt.Printf("[DEMO] Local files: %d\n", len(files))
					for _, file := range files {
						fmt.Printf("[DEMO]   - %s (size: %d)\n", file.Path, file.Size)
					}
				}
			}
		}()

		// Keep running in demo mode
		select {}
	}
}
