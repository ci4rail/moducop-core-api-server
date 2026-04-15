/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
	"github.com/ci4rail/moducop-core-api-server/internal/manager/cpumanager"
	"github.com/ci4rail/moducop-core-api-server/internal/manager/io4edgemanager"
	"github.com/ci4rail/moducop-core-api-server/internal/updatestore"
)

const (
	apiPrefix                   = "/api/v1"
	readHeaderTO                = 5 * time.Second
	readTO                      = 300 * time.Second
	// PUT update endpoints read the full artifact before they can return a response.
	// Keep the write timeout above the upload read budget so slow real-target uploads
	// can still complete and return a proper JSON error/status response.
	writeTO                     = readTO + 30*time.Second
	idleTO                      = 60 * time.Second
	errUnknown                  = "api-0000"
	errCodeBadRequest           = "api-0001"
	errCodeCreateTempFailed     = "api-0002"
	errCodeReadBodyFailed       = "api-0003"
	errCodeFinalizeFileFailed   = "api-0004"
	errCodeWriteErrorJSONFailed = "api-0005"
	errCodeEeInvFailed          = "api-0006"
)

type API struct {
	cpuManager     *cpumanager.CPUManager
	io4edgeManager *io4edgemanager.Io4edgeManager
	logger         *loglite.Logger
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var errCommandFailed = errors.New("command failed")

func Start(address string, cpuManager *cpumanager.CPUManager, io4edgeManager *io4edgemanager.Io4edgeManager, logLevel loglite.Level) {
	a := &API{
		cpuManager:     cpuManager,
		io4edgeManager: io4edgeManager,
		logger:         loglite.New("server", os.Stdout, logLevel),
	}
	handler := a.routes()
	if err := updatestore.EnsurePath(); err != nil {
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

func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT "+apiPrefix+"/software/core-os", a.handleLoadCoreOS)
	mux.HandleFunc("GET "+apiPrefix+"/software/core-os", a.handleGetCoreOS)
	mux.HandleFunc("PUT "+apiPrefix+"/software/application/{applicationname}", a.handleLoadApplication)
	mux.HandleFunc("GET "+apiPrefix+"/software/application/{applicationname}", a.handleGetApplication)
	mux.HandleFunc("GET "+apiPrefix+"/software/applications", a.handleListApplications)
	mux.HandleFunc("POST "+apiPrefix+"/system/reboot", a.handleReboot)
	mux.HandleFunc("GET "+apiPrefix+"/software/io4edge/{devicename}", a.handleGetIo4EdgeSoftware)
	mux.HandleFunc("PUT "+apiPrefix+"/software/io4edge/{devicename}", a.handleLoadIo4EdgeSoftware)
	mux.HandleFunc("GET "+apiPrefix+"/hardware", a.handleGetHardwareInfo)
	mux.HandleFunc("GET "+apiPrefix+"/hardware/io4edge-devices", a.handleListIo4EdgeDevices)
	return mux
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

func (a *API) writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		a.logger.Errorf("%s: failed to encode response payload: %v", errCodeWriteErrorJSONFailed, err)
	}
}
