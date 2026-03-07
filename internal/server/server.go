package server

import (
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
	"github.com/ci4rail/moducop-core-api-server/internal/manager/cpumanager"
	"github.com/ci4rail/moducop-core-api-server/internal/prefixfs"
)

const (
	apiPrefix      = "/api/v1"
	updateFilePath = "/data/core-api-server/updates/"
	dirModeDefault = 0o755
	readHeaderTO   = 5 * time.Second
	readTO         = 30 * time.Second
	writeTO        = 30 * time.Second
	idleTO         = 60 * time.Second
)

type API struct {
	cpuManager *cpumanager.CPUManager
	logger     *loglite.Logger
}

func Start(address string, cpuManager *cpumanager.CPUManager, logLevel loglite.Level) {
	a := &API{
		cpuManager: cpuManager,
		logger:     loglite.New("server", os.Stdout, logLevel),
	}
	handler := a.routes()
	if err := ensureUpdateFilePath(); err != nil {
		a.logger.Errorf("failed to create update file path: %v", err)
		panic(err)
	}
	go func() {
		a.logger.Infof("starting server on %s", address)
		srv := &http.Server{
			Addr:              address,
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTO,
			ReadTimeout:       readTO,
			WriteTimeout:      writeTO,
			IdleTimeout:       idleTO,
		}
		if err := srv.ListenAndServe(); err != nil {
			a.logger.Errorf("server failed: %v", err)
			panic(err)
		}
	}()
}

func ensureUpdateFilePath() error {
	return os.MkdirAll(getUpdateFilePath(), dirModeDefault)
}

func getUpdateFilePath() string {
	return prefixfs.Path(updateFilePath)
}

func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT "+apiPrefix+"/software/application/{applicationname}", a.handleLoadApplication)
	return mux
}

func (a *API) handleLoadApplication(w http.ResponseWriter, r *http.Request) {
	appName := r.PathValue("applicationname")
	if appName == "" {
		http.Error(w, "application name is required", http.StatusBadRequest)
		return
	}
	tmp, err := os.CreateTemp(getUpdateFilePath(), "app-*")
	if err != nil {
		http.Error(w, "failed to create temporary file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	a.logger.Infof("received request to update application %s. Download to %s", appName, tmp.Name())

	tmpPath := tmp.Name()
	keepFile := false
	defer func() {
		_ = tmp.Close()
		if !keepFile {
			_ = os.Remove(tmpPath)
		}
	}()

	_, err = io.Copy(tmp, r.Body)
	if err != nil {
		http.Error(w, "failed to read request body: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tmp.Close(); err != nil {
		http.Error(w, "failed to finalize update file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	reply := make(chan cpumanager.Result[struct{}], 1)
	_, err = cpumanager.Ask(r.Context(), a.cpuManager,
		cpumanager.StartApplicationUpdate{
			AppName:          appName,
			PathToMenderFile: tmpPath,
			Reply:            reply,
		},
		reply,
	)
	if err != nil {
		http.Error(w, "failed to start application update: "+err.Error(), http.StatusInternalServerError)
		return
	}
	a.logger.Infof("started application update for %s", appName)
	keepFile = true
	w.WriteHeader(http.StatusAccepted)
}
