package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
)

type startNamedUpdateFn func(ctx context.Context, name, tmpPath string) (status int, code, message string, err error)

func (a *API) handleLoadNamedUpdate(
	w http.ResponseWriter,
	r *http.Request,
	pathParam, requiredErrorMessage, tempPatternFmt, startLogFmt string,
	startFn startNamedUpdateFn,
) {
	name := r.PathValue(pathParam)
	if name == "" {
		a.writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, requiredErrorMessage)
		return
	}
	tmpPath, errCode, err := a.saveBodyToFile(r.Body, fmt.Sprintf(tempPatternFmt, name))
	if err != nil {
		a.writeJSONError(w, http.StatusInternalServerError, errCode, fmt.Sprintf("failed to save update file: %v", err))
		return
	}
	keepFile := false
	defer func() {
		if !keepFile {
			if rmErr := os.Remove(tmpPath); rmErr != nil {
				a.logger.Errorf("failed to remove temporary file %s: %v", tmpPath, rmErr)
			}
		}
	}()

	status, code, message, err := startFn(r.Context(), name, tmpPath)
	if err != nil {
		a.writeJSONError(w, status, code, message)
		return
	}
	a.logger.Infof(startLogFmt, name)
	keepFile = true
	w.WriteHeader(http.StatusAccepted)
}
