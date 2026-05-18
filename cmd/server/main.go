package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/cli"
	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/internal/l10n"
	"github.com/Pimeng/gphira-mp-next/internal/network"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "path to configuration file")

	// CLI flags that map to config values
	var (
		cliHost         = flag.String("host", "", "listen host address")
		cliPort         = flag.Int("port", 0, "listen port")
		cliHTTPService  = flag.String("httpService", "", "enable HTTP service (true/false)")
		cliHTTPPort     = flag.Int("httpPort", 0, "HTTP service port")
		cliRoomMaxUsers = flag.Int("roomMaxUsers", 0, "max users per room (1-64)")
		cliServerName   = flag.String("serverName", "", "server display name")
		cliMonitors     = flag.String("monitors", "", "monitor user IDs (comma-separated)")
	)

	flag.Parse()

	// Auto-discover config file if not explicitly specified
	if configPath == "" {
		if _, err := os.Stat("server_config.yml"); err == nil {
			configPath = "server_config.yml"
		} else {
			configPath = "config.yaml"
		}
	}

	// Build configuration with priority: defaults < file < environment < CLI
	buildConfig := func() (*config.ServerConfig, error) {
		cfg := config.DefaultConfig()

		if _, err := os.Stat(configPath); err == nil {
			loaded, err := config.LoadConfig(configPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load config file: %w", err)
			}
			cfg = config.MergeConfig(cfg, loaded)
		}

		envCfg := config.LoadEnvConfig()
		cfg = config.MergeConfig(cfg, envCfg)

		var cliErrors []string
		flag.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "host":
				if v := strings.TrimSpace(*cliHost); v != "" {
					cfg.Host = v
				}
			case "port":
				if *cliPort != 0 {
					if *cliPort < 1 || *cliPort > 65535 {
						cliErrors = append(cliErrors, "invalid port: must be 1-65535")
					} else {
						cfg.Port = *cliPort
					}
				}
			case "httpService":
				if v, ok := config.ParseBool(strings.TrimSpace(*cliHTTPService)); ok {
					cfg.HTTPService = &v
				} else {
					cliErrors = append(cliErrors, "invalid httpService: must be true/false/1/0/yes/no/on/off")
				}
			case "httpPort":
				if *cliHTTPPort != 0 {
					if *cliHTTPPort < 1 || *cliHTTPPort > 65535 {
						cliErrors = append(cliErrors, "invalid httpPort: must be 1-65535")
					} else {
						cfg.HTTPPort = *cliHTTPPort
					}
				}
			case "roomMaxUsers":
				if *cliRoomMaxUsers != 0 {
					if *cliRoomMaxUsers < 1 {
						cliErrors = append(cliErrors, "invalid roomMaxUsers: must be >= 1")
					} else {
						v := *cliRoomMaxUsers
						if v > 64 {
							v = 64
						}
						cfg.RoomMaxUsers = v
					}
				}
			case "serverName":
				if v := strings.TrimSpace(*cliServerName); v != "" {
					cfg.ServerName = v
				}
			case "monitors":
				if v, ok := config.ParseIntegerList(strings.TrimSpace(*cliMonitors)); ok {
					cfg.Monitors = v
				} else {
					cliErrors = append(cliErrors, "invalid monitors: must be comma-separated integers")
				}
			}
		})

		if len(cliErrors) > 0 {
			for _, err := range cliErrors {
				fmt.Fprintf(os.Stderr, "CLI error: %s\n", err)
			}
			return nil, fmt.Errorf("invalid CLI flags")
		}
		return cfg, nil
	}

	cfg, err := buildConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	logger := utils.NewLogger(cfg.LogLevel)
	logger.SetRateLimiter(utils.NewRateLimiter(10, time.Minute, 5*time.Minute))
	logger.SetTestAccountIDs(cfg.TestAccountIDs)

	lang := l10n.New(cfg.Lang)

	// Initialize Redis if configured
	if cfg.Redis != nil && cfg.Redis.Enabled {
		if err := utils.InitRedis(cfg.Redis); err != nil {
			logger.Warn("failed to initialize Redis cache", "err", err)
		} else {
			logger.Mark(lang.Format("log-redis-enabled", nil))
		}
	}

	logger.Mark(lang.Format("log-server-starting", nil))

	var server *network.Server
	startServer := func() error {
		var err error
		server, err = network.StartServer(cfg, logger, configPath)
		return err
	}

	restartServer := func() error {
		logger.Mark(lang.Format("log-restarting-server", nil))
		if server != nil {
			if err := server.Close(); err != nil {
				logger.Error("error closing server for restart", "err", err)
			}
		}
		newCfg, err := buildConfig()
		if err != nil {
			return err
		}
		cfg = newCfg
		logger.UpdateOptions(cfg.LogLevel, cfg.TestAccountIDs)
		lang = l10n.New(cfg.Lang)
		return startServer()
	}

	if err := startServer(); err != nil {
		logger.Error("failed to start server", "err", err)
		os.Exit(1)
	}

	// Start CLI
	c := cli.NewCLI(server.State(), logger, func(cmd protocol.ServerCommand) error {
		server.BroadcastAll(cmd)
		return nil
	}, func() error {
		return server.Close()
	}, func(id int32) bool {
		// kick user by marking their session as lost
		var kicked bool
		server.State().WithRLock(func() {
			if u := server.State().Users[id]; u != nil {
				if s, ok := u.Session.(*network.Session); ok {
					s.MarkLost()
					kicked = true
				}
			}
		})
		return kicked
	}, restartServer, func() *state.ServerState { return server.State() })
	c.Start()

	// Wait for interrupt signal or CLI stop command
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sigCh:
	case <-c.Done():
	}

	logger.Mark(lang.Format("log-shutting-down", nil))
	c.Stop()
	utils.CloseRedis()
	if err := server.Close(); err != nil {
		logger.Error("error during shutdown", "err", err)
	}
}
