package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/mqtt-home/mqtt-sonos/bridge"
	"github.com/mqtt-home/mqtt-sonos/config"
	"github.com/mqtt-home/mqtt-sonos/sonos"
	"github.com/mqtt-home/mqtt-sonos/version"
	"github.com/mqtt-home/mqtt-sonos/web"
	"github.com/philipparndt/go-logger"
)

func main() {
	logger.Init("info", logger.Logger())
	logger.Info("mqtt-sonos", "version", version.Info())
	initPprof()

	if len(os.Args) < 2 {
		logger.Error("No configuration file specified")
		os.Exit(1)
	}

	configFile := os.Args[1]
	logger.Info("Configuration file", "path", configFile)

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	logger.SetLevel(cfg.LogLevel)

	mgr := sonos.NewManager(cfg.Sonos, cfg.Sonos.Device)
	b := bridge.New(cfg, mgr)

	var webServer *web.WebServer
	if cfg.Web.Enabled {
		webServer = web.NewWebServer(mgr)
		b.SetStateListener(func(uuid string, state *sonos.State) {
			webServer.BroadcastState(uuid, state)
		})
	}

	if err := b.Start(); err != nil {
		logger.Error("Failed to start Sonos bridge", "error", err)
		os.Exit(1)
	}

	if webServer != nil {
		go func() {
			port := cfg.Web.Port
			logger.Info("Web interface available", "url", "http://localhost:"+strconv.Itoa(port))
			if err := webServer.Start(port); err != nil {
				logger.Error("Failed to start web server", "error", err)
			}
		}()
	}

	logger.Info("Application ready")

	quitChannel := make(chan os.Signal, 1)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)
	<-quitChannel

	b.Stop()
	logger.Info("Shutdown complete")
}

func initPprof() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
}
