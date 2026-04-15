/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ci4rail/moducop-core-api-server/internal/updatestore"
)

var errRemoveOutsideUpdateDir = errors.New("refusing to remove file outside update directory")

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
			if rmErr := removeTempUpdateFile(tmpPath); rmErr != nil {
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

// Save body to a temporary file and return the path to the file.
// The caller is responsible for deleting the file when it is no longer needed.
// Return path, errCode, error
func (a *API) saveBodyToFile(body io.Reader, tempPattern string) (string, string, error) {
	tmp, err := os.CreateTemp(updatestore.GetPath(), tempPattern)
	if err != nil {
		return "", errCodeCreateTempFailed, fmt.Errorf("failed to create temporary file: %w", err)
	}
	a.logger.Debugf("Download to %s", tmp.Name())

	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
	}()

	_, err = io.Copy(tmp, body)
	if err != nil {
		return "", errCodeReadBodyFailed, fmt.Errorf("failed to read request body: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return "", errCodeFinalizeFileFailed, fmt.Errorf("failed to finalize update file: %w", err)
	}

	return tmpPath, "", nil
}

func removeTempUpdateFile(tmpPath string) error {
	baseDir := filepath.Clean(updatestore.GetPath())
	cleanPath := filepath.Clean(tmpPath)
	prefix := baseDir + string(filepath.Separator)
	if !strings.HasPrefix(cleanPath, prefix) {
		return fmt.Errorf("%w: %s", errRemoveOutsideUpdateDir, cleanPath)
	}
	return os.Remove(cleanPath)
}
