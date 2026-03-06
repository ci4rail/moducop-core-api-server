package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
	"github.com/ci4rail/moducop-core-api-server/internal/manager/cpumanager"
	"github.com/ci4rail/moducop-core-api-server/internal/prefixfs"
	"github.com/ci4rail/moducop-core-api-server/internal/server"
)

const (
	defaultServerAddress = ":8080"
	persistentStatePath  = "/data/core-api-server/cpumanager-state.json"
)

func main() {
	serverAddress := flag.String("server.address", defaultServerAddress, "HTTP server listen address")
	logLevelFlag := flag.String("log.level", "info", "Log level: debug|info|warn|error|off")
	flag.Parse()

	logLevel, err := parseLogLevel(*logLevelFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --log.level: %v\n", err)
		os.Exit(2)
	}

	statePath := prefixfs.Path(persistentStatePath)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create state directory: %v\n", err)
		os.Exit(1)
	}

	cpuManager, err := cpumanager.New(statePath, logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize cpu manager: %v\n", err)
		os.Exit(1)
	}

	server.Start(*serverAddress, cpuManager)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}

func parseLogLevel(s string) (loglite.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return loglite.Debug, nil
	case "info":
		return loglite.Info, nil
	case "warn":
		return loglite.Warn, nil
	case "error":
		return loglite.Error, nil
	case "off":
		return loglite.Off, nil
	default:
		return loglite.Info, errors.New("expected one of debug|info|warn|error|off")
	}
}
