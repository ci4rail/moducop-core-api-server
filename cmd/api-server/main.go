package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ci4rail/moducop-core-api-server/internal/buildinfo"
	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
	"github.com/ci4rail/moducop-core-api-server/internal/manager/cpumanager"
	"github.com/ci4rail/moducop-core-api-server/internal/manager/io4edgemanager"
	"github.com/ci4rail/moducop-core-api-server/internal/prefixfs"
	"github.com/ci4rail/moducop-core-api-server/internal/server"
)

const (
	defaultServerAddress              = ":8090"
	cpuManagerPersistentStatePath     = "/data/core-api-server/cpumanager-state.json"
	io4edgeManagerPersistentStatePath = "/data/core-api-server/io4edgemanager-state.json"
	exitCodeUsageError                = 2
	dirModeDefault                    = 0o755
)

type staticError string

func (e staticError) Error() string { return string(e) }

const errInvalidLogLevel staticError = "expected one of debug|info|warn|error|off"

func main() {
	serverAddress := flag.String("server.address", defaultServerAddress, "HTTP server listen address")
	logLevelFlag := flag.String("log.level", "info", "Log level: debug|info|warn|error|off")
	printVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *printVersion {
		fmt.Println(buildinfo.Version)
		return
	}

	logLevel, err := parseLogLevel(*logLevelFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --log.level: %v\n", err)
		os.Exit(exitCodeUsageError)
	}
	loglite.New("main", os.Stdout, logLevel).Infof("starting moducop-core-api-server version %s", buildinfo.Version)

	statePath := prefixfs.Path(cpuManagerPersistentStatePath)
	if err := os.MkdirAll(filepath.Dir(statePath), dirModeDefault); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create state directory: %v\n", err)
		os.Exit(1)
	}

	cpuManager, err := cpumanager.New(statePath, logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize cpu manager: %v\n", err)
		os.Exit(1)
	}

	io4edgeManager, err := io4edgemanager.New(prefixfs.Path(io4edgeManagerPersistentStatePath), logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize io4edge manager: %v\n", err)
		os.Exit(1)
	}

	server.Start(*serverAddress, cpuManager, io4edgeManager, logLevel)

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
		return loglite.Info, errInvalidLogLevel
	}
}
