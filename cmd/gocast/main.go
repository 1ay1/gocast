// GoCast - A modern Icecast replacement written in Go
package main

import (
	"context"
	"flag"
	"fmt"
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

var userAgent = "GoCast/" + version

func main() {
	// Parse command line flags
	configFile := flag.String("config", "gocast.vibe", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	showHelp := flag.Bool("help", false, "Show help message")
	checkConfig := flag.Bool("check", false, "Check configuration file and exit")

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

	// Setup logging
	logger := log.New(os.Stdout, "[GoCast] ", log.LstdFlags|log.Lmsgprefix)

	// Print banner
	printBanner(logger)

	// Load configuration
	cfg, err := loadConfig(*configFile, logger)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Fatalf("Invalid configuration: %v", err)
	}

	if *checkConfig {
		logger.Println("Configuration OK")
		os.Exit(0)
	}

	// Create and start server
	srv := server.New(cfg, logger)

	if err := srv.Start(); err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}

	logger.Printf("GoCast is running on http://%s:%d", cfg.Server.Hostname, cfg.Server.Port)
	if cfg.Server.SSLEnabled {
		logger.Printf("HTTPS enabled on port %d", cfg.Server.SSLPort)
	}
	logger.Printf("Admin interface: http://%s:%d%s/", cfg.Server.Hostname, cfg.Server.Port, cfg.Server.AdminRoot)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-quit

		switch sig {
		case syscall.SIGHUP:
			// Reload configuration
			logger.Println("Received SIGHUP, reloading configuration...")
			newCfg, err := loadConfig(*configFile, logger)
			if err != nil {
				logger.Printf("Failed to reload configuration: %v", err)
				continue
			}
			if err := newCfg.Validate(); err != nil {
				logger.Printf("Invalid configuration: %v", err)
				continue
			}
			logger.Println("Configuration reloaded successfully")
			// Note: Full hot-reload would require more complex implementation
			// For now, suggest restart for changes to take effect

		case syscall.SIGINT, syscall.SIGTERM:
			logger.Printf("Received %v, shutting down...", sig)

			// Create shutdown context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

// loadConfig loads configuration from file or creates default
func loadConfig(filename string, logger *log.Logger) (*config.Config, error) {
	// Check if config file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		logger.Printf("Configuration file not found: %s", filename)
		logger.Println("Using default configuration")
		return config.DefaultConfig(), nil
	}

	logger.Printf("Loading configuration from %s", filename)
	cfg, err := config.Load(filename)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// printBanner prints the GoCast startup banner
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

// printUsage prints help message
func printUsage() {
	fmt.Printf(`GoCast %s - A modern Icecast replacement written in Go

USAGE:
    gocast [OPTIONS]

OPTIONS:
    -config <file>    Path to configuration file (default: gocast.vibe)
    -check            Check configuration file and exit
    -version          Show version information
    -help             Show this help message

CONFIGURATION:
    GoCast uses the VIBE configuration format. Example:

    # Server settings
    server {
        hostname localhost
        port 8000
        location "Earth"
    }

    # Authentication
    auth {
        source_password hackme
        admin_user admin
        admin_password hackme
    }

    # Limits
    limits {
        max_clients 100
        max_sources 10
        queue_size 524288
        burst_size 65535
    }

    # Mount points
    mounts {
        live {
            password secret
            max_listeners 100
            genre "Various"
            description "Live Stream"
        }
    }

SIGNALS:
    SIGINT, SIGTERM   Graceful shutdown
    SIGHUP            Reload configuration

For more information, visit: https://github.com/gocast/gocast
`, version)
}
