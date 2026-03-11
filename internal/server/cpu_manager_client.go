package server

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/ci4rail/moducop-core-api-server/internal/manager/cpumanager"
)

func (a *API) handleLoadCoreOS(w http.ResponseWriter, r *http.Request) {
	tmpPath, errCode, err := a.saveBodyToFile(r.Body, "coreos-*")
	if err != nil {
		a.writeJSONError(w, http.StatusInternalServerError, errCode, fmt.Sprintf("failed to save update file: %v", err))
		return
	}
	keepFile := false
	defer func() {
		if !keepFile {
			if err := os.Remove(tmpPath); err != nil {
				a.logger.Errorf("failed to remove temporary file %s: %v", tmpPath, err)
			}
		}
	}()

	reply := make(chan cpumanager.Result[struct{}], 1)
	_, code, message, err := execCPUManagerCommand(r.Context(), a.cpuManager,
		cpumanager.StartCoreOsUpdate{
			PathToMenderFile: tmpPath,
			Reply:            reply,
		},
		reply,
	)
	if err != nil {
		a.writeJSONError(w, statusFromCPUManagerCode(code), code, message)
		return
	}
	a.logger.Infof("started core OS update")
	keepFile = true
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) handleGetCoreOS(w http.ResponseWriter, r *http.Request) {
	reply := make(chan cpumanager.Result[cpumanager.EntityStatus], 1)
	res, code, message, err := execCPUManagerCommand(r.Context(), a.cpuManager,
		cpumanager.GetCoreOsState{
			Reply: reply,
		},
		reply,
	)
	if err != nil {
		a.writeJSONError(w, statusFromCPUManagerCode(code), code, message)
		return
	}
	a.writeJSON(w, http.StatusOK, res)
}

func (a *API) handleLoadApplication(w http.ResponseWriter, r *http.Request) {
	appName := r.PathValue("applicationname")
	if appName == "" {
		a.writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, "application name is required")
		return
	}
	tmpPath, errCode, err := a.saveBodyToFile(r.Body, fmt.Sprintf("app-%s-*", appName))
	if err != nil {
		a.writeJSONError(w, http.StatusInternalServerError, errCode, fmt.Sprintf("failed to save update file: %v", err))
		return
	}
	keepFile := false
	defer func() {
		if !keepFile {
			if err := os.Remove(tmpPath); err != nil {
				a.logger.Errorf("failed to remove temporary file %s: %v", tmpPath, err)
			}
		}
	}()

	reply := make(chan cpumanager.Result[struct{}], 1)
	_, code, message, err := execCPUManagerCommand(r.Context(), a.cpuManager,
		cpumanager.StartApplicationUpdate{
			AppName:          appName,
			PathToMenderFile: tmpPath,
			Reply:            reply,
		},
		reply,
	)
	if err != nil {
		a.writeJSONError(w, statusFromCPUManagerCode(code), code, message)
		return
	}
	a.logger.Infof("started application update for %s", appName)
	keepFile = true
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) handleGetApplication(w http.ResponseWriter, r *http.Request) {
	appName := r.PathValue("applicationname")
	if appName == "" {
		a.writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, "application name is required")
		return
	}
	reply := make(chan cpumanager.Result[cpumanager.EntityStatus], 1)
	res, code, message, err := execCPUManagerCommand(r.Context(), a.cpuManager,
		cpumanager.GetApplicationState{
			AppName: appName,
			Reply:   reply,
		},
		reply,
	)
	if err != nil {
		a.writeJSONError(w, statusFromCPUManagerCode(code), code, message)
		return
	}
	a.writeJSON(w, http.StatusOK, res)
}

func (a *API) handleListApplications(w http.ResponseWriter, r *http.Request) {
	reply := make(chan cpumanager.Result[[]string], 1)
	res, code, message, err := execCPUManagerCommand(r.Context(), a.cpuManager,
		cpumanager.ListApplications{
			Reply: reply,
		},
		reply,
	)
	if err != nil {
		a.writeJSONError(w, statusFromCPUManagerCode(code), code, message)
		return
	}
	a.writeJSON(w, http.StatusOK, res)
}

func (a *API) handleReboot(w http.ResponseWriter, r *http.Request) {
	reply := make(chan cpumanager.Result[struct{}], 1)
	_, code, message, err := execCPUManagerCommand(r.Context(), a.cpuManager,
		cpumanager.Reboot{
			Reply: reply,
		},
		reply,
	)
	if err != nil {
		a.writeJSONError(w, statusFromCPUManagerCode(code), code, message)
		return
	}
	a.writeJSON(w, http.StatusOK, struct{}{})
}

func execCPUManagerCommand[T any, C cpumanager.Command](ctx context.Context, m *cpumanager.CPUManager, cmd C, reply chan cpumanager.Result[T]) (T, string, string, error) {
	res, err := cpumanager.Ask(ctx, m, cmd, reply)
	if err != nil {
		if code, message, ok := cpumanager.ExtractCode(err); ok {
			return res, code, message, fmt.Errorf("command failed: %s: %s", code, message)
		}
		return res, errUnknown, "", fmt.Errorf("command failed: %w", err)
	}
	return res, "", "", err
}

func statusFromCPUManagerCode(code string) int {
	switch code {
	case cpumanager.ErrCodeAlreadyDeployed:
		return http.StatusConflict
	case cpumanager.ErrCodeArtifactInvalid:
		return http.StatusBadRequest
	case cpumanager.ErrCodeInvalidCoreOSEntityName:
		return http.StatusBadRequest
	case cpumanager.ErrCodeEntityUpdateInProgress, cpumanager.ErrCodeMenderBusy:
		return http.StatusPreconditionFailed
	default:
		return http.StatusInternalServerError
	}
}
