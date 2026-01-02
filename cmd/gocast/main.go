// GoCast - A modern Icecast replacement written in Go
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gocast/gocast/internal/config"
	"github.com/gocast/gocast/internal/server"
)

// Version information - injected at build time via ldflags
var (
	version   = "dev"
	gitCommit = "unknown"
	buildDate = "unknown"
)

func main() {
	// Parse command line flags
	configFile := flag.String("config", "", "Path to configuration file (optional)")
	dataDir := flag.String("data", "", "Data directory for persistent config (default: auto-detect)")
	showVersion := flag.Bool("version", false, "Show version information")
	showHelp := flag.Bool("help", false, "Show help message")

	flag.Parse()

	if *showHelp {
		printUsage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Printf("GoCast %s\n", version)
		fmt.Printf("  Git Commit: %s\n", gitCommit)
		fmt.Printf("  Build Date: %s\n", buildDate)
		fmt.Println("  https://github.com/gocast/gocast")
		os.Exit(0)
	}

	// Setup initial logging to stdout
	logger := log.New(os.Stdout, "[GoCast] ", log.LstdFlags|log.Lmsgprefix)

	// Print banner
	printBanner(logger)

	var srv *server.Server
	var cfg *config.Config

	// Check if config file is provided
	if *configFile != "" {
		// Legacy mode: use config file
		logger.Printf("Loading configuration from %s", *configFile)

		configManager, err := config.NewConfigManagerWithLogger(*configFile, logger)
		if err != nil {
			logger.Fatalf("Failed to load configuration: %v", err)
		}

		cfg = configManager.GetConfig()

		if err := cfg.Validate(); err != nil {
			logger.Fatalf("Invalid configuration: %v", err)
		}

		if configManager.HasStateOverrides() {
			logger.Println("Runtime configuration overrides loaded from state.json")
		}

		srv = server.NewWithConfigManager(configManager, logger)
	} else {
		// Zero-config mode: all settings via admin panel
		logger.Println("Starting in zero-config mode...")

		var err error
		srv, err = server.NewWithSetupManager(*dataDir, logger)
		if err != nil {
			logger.Fatalf("Failed to initialize server: %v", err)
		}

		cfg = srv.GetConfigManager().GetConfig()
	}

	// Setup log capture for admin panel
	logWriter := srv.GetLogWriter("server")
	if logWriter != nil {
		multiWriter := io.MultiWriter(os.Stdout, logWriter)
		logger.SetOutput(multiWriter)

		stderrWriter := srv.GetLogWriter("stderr")
		if stderrWriter != nil {
			stderrMulti := io.MultiWriter(os.Stderr, stderrWriter)
			log.SetOutput(stderrMulti)
		}

		logger.Println("Log capture enabled for admin panel")
	}

	// Start the server
	if err := srv.Start(); err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}

	// Print server info
	if cfg.Server.AutoSSL {
		logger.Printf("GoCast is running with AutoSSL on https://%s", cfg.Server.Hostname)
		logger.Printf("HTTP redirect active on port 80")
	} else if cfg.Server.SSLEnabled {
		logger.Printf("GoCast is running on https://%s:%d", cfg.Server.Hostname, cfg.Server.SSLPort)
	} else {
		logger.Printf("GoCast is running on http://%s:%d", cfg.Server.Hostname, cfg.Server.Port)
	}

	logger.Printf("Admin panel: http://%s:%d/admin/", cfg.Server.Hostname, cfg.Server.Port)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-quit

		switch sig {
		case syscall.SIGHUP:
			logger.Println("Received SIGHUP, reloading configuration...")
			// TODO: Implement hot reload for setup manager mode
			logger.Println("Configuration reload not yet implemented in zero-config mode")

		case syscall.SIGINT, syscall.SIGTERM:
			logger.Printf("Received %v, shutting down...", sig)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := srv.Stop(ctx); err != nil {
				logger.Printf("Error during shutdown: %v", err)
				os.Exit(1)
			}

			logger.Println("GoCast shutdown complete")
			os.Exit(0)
		}
	}
}

func printBanner(logger *log.Logger) {
	banner := `
   ██████╗  ██████╗  ██████╗ █████╗ ███████╗████████╗
  ██╔════╝ ██╔═══██╗██╔════╝██╔══██╗██╔════╝╚══██╔══╝
  ██║  ███╗██║   ██║██║     ███████║███████╗   ██║
  ██║   ██║██║   ██║██║     ██╔══██║╚════██║   ██║
  ╚██████╔╝╚██████╔╝╚██████╗██║  ██║███████║   ██║
   ╚═════╝  ╚═════╝  ╚═════╝╚═╝  ╚═╝╚══════╝   ╚═╝

  Modern Icecast Replacement - v%s
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`
	fmt.Printf(banner, version)
}

func printUsage() {
	fmt.Printf(`GoCast %s - A modern Icecast replacement written in Go

USAGE:
    gocast [OPTIONS]

OPTIONS:
    -data <dir>       Data directory for persistent config (default: auto-detect)
    -config <file>    Path to configuration file (optional, legacy mode)
    -version          Show version information
    -help             Show this help message

ZERO-CONFIG MODE (Default):
    GoCast runs without any configuration file. On first start, it will:
    1. Generate secure admin credentials (shown once in console)
    2. Start the admin panel at http://localhost:8000/admin/
    3. All settings are configured via the web admin panel

    Configuration is automatically persisted to the data directory.

LEGACY MODE (with -config):
    Use a VIBE configuration file for settings.

SIGNALS:
    SIGINT, SIGTERM   Graceful shutdown
    SIGHUP            Reload configuration

For more information, visit: https://github.com/gocast/gocast
`, version)
}
