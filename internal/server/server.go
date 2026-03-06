package server

import (
	"io"
	"net/http"
	"os"

	"github.com/ci4rail/moducop-core-api-server/internal/manager/cpumanager"
	"github.com/ci4rail/moducop-core-api-server/internal/prefixfs"
)

const (
	apiPrefix      = "/api/v1"
	updateFilePath = "/data/core-api-server/updates/"
)

type API struct {
	cpuManager *cpumanager.CpuManager
}

func Start(address string, cpuManager *cpumanager.CpuManager) {
	a := &API{
		cpuManager: cpuManager,
	}
	handler := a.routes()
	ensureUpdateFilePath()
	go func() {
		if err := http.ListenAndServe(address, handler); err != nil {
			panic(err)
		}
	}()
}

func ensureUpdateFilePath() error {
	return os.MkdirAll(getUpdateFilePath(), 0755)
}

func getUpdateFilePath() string {
	return prefixfs.Path(updateFilePath)
}

func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+apiPrefix+"/software/application/{application-name}", a.handleLoadApplication)
	return mux
}

func (a *API) handleLoadApplication(w http.ResponseWriter, r *http.Request) {
	appName := r.PathValue("application-name")
	if appName == "" {
		http.Error(w, "application name is required", http.StatusBadRequest)
		return
	}

	tmp, err := os.CreateTemp(getUpdateFilePath(), "app-*")
	if err != nil {
		http.Error(w, "failed to create temporary file: "+err.Error(), http.StatusInternalServerError)
		return
	}

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
	_, err = cpumanager.Ask(r.Context(), a.cpuManager, cpumanager.StartApplicationUpdate{
		AppName: appName,
		PathToMenderFile: tmpPath,
		Reply:            reply,
	},
	reply,
	)
	if err != nil {
		http.Error(w, "failed to start application update: "+err.Error(), http.StatusInternalServerError)
		return
	}

	keepFile = true
	w.WriteHeader(http.StatusAccepted)
}
