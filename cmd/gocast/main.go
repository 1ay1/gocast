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
	dataDir := flag.String("data", "", "Data directory for config and state (default: ~/.gocast)")
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

	// Create server with config manager
	srv, err := server.NewWithSetupManager(*dataDir, logger)
	if err != nil {
		logger.Fatalf("Failed to initialize server: %v", err)
	}

	cfg := srv.GetConfigManager().GetConfig()

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
	if cfg.SSL.AutoSSL {
		logger.Printf("GoCast is running with AutoSSL on https://%s", cfg.Server.Hostname)
		logger.Printf("HTTP redirect active on port 80")
	} else if cfg.SSL.Enabled {
		logger.Printf("GoCast is running on https://%s:%d", cfg.Server.Hostname, cfg.SSL.Port)
	} else {
		logger.Printf("GoCast is running on http://%s:%d", cfg.Server.Hostname, cfg.Server.Port)
	}

	logger.Printf("Admin panel: http://%s:%d/admin/", cfg.Server.Hostname, cfg.Server.Port)
	logger.Printf("Config file: %s", srv.GetConfigManager().GetConfigPath())

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-quit

		switch sig {
		case syscall.SIGHUP:
			logger.Println("Received SIGHUP, reloading configuration...")
			if err := srv.GetConfigManager().Reload(); err != nil {
				logger.Printf("Failed to reload configuration: %v", err)
			} else {
				logger.Println("Configuration reloaded successfully")
			}

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
    -data <dir>       Data directory for config and state (default: ~/.gocast)
    -version          Show version information
    -help             Show this help message

HOW IT WORKS:
    GoCast stores all configuration in a single JSON file (~/.gocast/config.json).

    On first start:
    1. Secure admin credentials are generated (shown once in console)
    2. A default /live mount point is created
    3. The admin panel is available at http://localhost:8000/admin/

    All settings can be changed via the web admin panel. Changes are saved
    automatically and most take effect immediately (no restart needed).

SIGNALS:
    SIGINT, SIGTERM   Graceful shutdown
    SIGHUP            Hot reload configuration from disk

CONFIGURATION:
    Config file: ~/.gocast/config.json

    You can edit this file directly - use SIGHUP or the admin panel's
    "Reload from Disk" button to apply changes without restarting.

For more information, visit: https://github.com/gocast/gocast
`, version)
}
