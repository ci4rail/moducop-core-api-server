package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
	"github.com/ci4rail/moducop-core-api-server/internal/manager/cpumanager"
	"github.com/ci4rail/moducop-core-api-server/internal/prefixfs"
)

const (
	apiPrefix                   = "/api/v1"
	updateFilePath              = "/data/core-api-server/updates/"
	dirModeDefault              = 0o755
	readHeaderTO                = 5 * time.Second
	readTO                      = 30 * time.Second
	writeTO                     = 30 * time.Second
	idleTO                      = 60 * time.Second
	errCodeBadRequest           = "gen-0001"
	errCodeCreateTempFailed     = "gen-0002"
	errCodeReadBodyFailed       = "gen-0003"
	errCodeFinalizeFileFailed   = "gen-0004"
	errCodeStartUpdateFailed    = "gen-0005"
	errCodeWriteErrorJSONFailed = "gen-0006"
)

type API struct {
	cpuManager *cpumanager.CPUManager
	logger     *loglite.Logger
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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
		a.writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, "application name is required")
		return
	}
	tmp, err := os.CreateTemp(getUpdateFilePath(), "app-*")
	if err != nil {
		a.writeJSONError(w, http.StatusInternalServerError, errCodeCreateTempFailed, "failed to create temporary file")
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
		a.writeJSONError(w, http.StatusInternalServerError, errCodeReadBodyFailed, "failed to read request body")
		return
	}

	if err := tmp.Close(); err != nil {
		a.writeJSONError(w, http.StatusInternalServerError, errCodeFinalizeFileFailed, "failed to finalize update file")
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
		if code, message, ok := cpumanager.ExtractCode(err); ok {
			a.writeJSONError(w, statusFromManagerCode(code), code, message)
			return
		}
		a.writeJSONError(w, http.StatusInternalServerError, errCodeStartUpdateFailed, "failed to start application update")
		return
	}
	a.logger.Infof("started application update for %s", appName)
	keepFile = true
	w.WriteHeader(http.StatusAccepted)
}

func statusFromManagerCode(code string) int {
	switch code {
	case cpumanager.ErrCodeInvalidCoreOSEntityName:
		return http.StatusBadRequest
	case cpumanager.ErrCodeEntityUpdateInProgress, cpumanager.ErrCodeMenderBusy:
		return http.StatusConflict
	case cpumanager.ErrCodeStartUpdateFailed:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func (a *API) writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := errorPayload{
		Code:    code,
		Message: message,
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		a.logger.Errorf("%s: failed to encode error payload: %v", errCodeWriteErrorJSONFailed, err)
	}
}
