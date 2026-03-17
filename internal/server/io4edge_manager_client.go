package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ci4rail/moducop-core-api-server/internal/manager/io4edgemanager"
)

func (a *API) handleLoadIo4EdgeSoftware(w http.ResponseWriter, r *http.Request) {
	a.handleLoadNamedUpdate(
		w,
		r,
		"devicename",
		"device name is required",
		"io4edge-%s-*",
		"started Io4Edge software update for %s",
		func(ctx context.Context, deviceName, tmpPath string) (int, string, string, error) {
			reply := make(chan io4edgemanager.Result[struct{}], 1)
			_, code, message, err := execIo4edgeManagerCommand(ctx, a.io4edgeManager,
				io4edgemanager.StartUpdate{
					DeviceName:  deviceName,
					PathToFWPKG: tmpPath,
					Reply:       reply,
				},
				reply,
			)
			if err != nil {
				return statusFromIO4EdgeManagerCode(code), code, message, err
			}
			return 0, "", "", nil
		},
	)
}

func (a *API) handleGetIo4EdgeSoftware(w http.ResponseWriter, r *http.Request) {
	deviceName := r.PathValue("devicename")
	if deviceName == "" {
		a.writeJSONError(w, http.StatusBadRequest, errCodeBadRequest, "device name is required")
		return
	}
	reply := make(chan io4edgemanager.Result[io4edgemanager.Io4edgeFWStatus], 1)
	res, code, message, err := execIo4edgeManagerCommand(r.Context(), a.io4edgeManager,
		io4edgemanager.GetState{
			DeviceName: deviceName,
			Reply:      reply,
		},
		reply,
	)
	if err != nil {
		a.writeJSONError(w, statusFromIO4EdgeManagerCode(code), code, message)
		return
	}
	a.writeJSON(w, res)
}

func (a *API) handleListIo4EdgeDevices(w http.ResponseWriter, r *http.Request) {
	reply := make(chan io4edgemanager.Result[[]string], 1)
	res, code, message, err := execIo4edgeManagerCommand(r.Context(), a.io4edgeManager,
		io4edgemanager.ListDeviceNames{
			Reply: reply,
		},
		reply,
	)
	if err != nil {
		a.writeJSONError(w, statusFromIO4EdgeManagerCode(code), code, message)
		return
	}
	a.writeJSON(w, res)
}

func execIo4edgeManagerCommand[T any, C io4edgemanager.Command](ctx context.Context, m *io4edgemanager.Io4edgeManager, cmd C, reply chan io4edgemanager.Result[T]) (T, string, string, error) {
	res, err := io4edgemanager.Ask(ctx, m, cmd, reply)
	if err != nil {
		if code, message, ok := io4edgemanager.ExtractCode(err); ok {
			return res, code, message, fmt.Errorf("%w: %s: %s", errCommandFailed, code, message)
		}
		return res, errUnknown, "", fmt.Errorf("%w: %w", errCommandFailed, err)
	}
	return res, "", "", err
}

func statusFromIO4EdgeManagerCode(code string) int {
	switch code {
	case io4edgemanager.ErrCodeAlreadyDeployed:
		return http.StatusConflict
	case io4edgemanager.ErrCodeArtifactInvalid, io4edgemanager.ErrCodeDeviceNotFound:
		return http.StatusBadRequest
	case io4edgemanager.ErrCodeDeviceUpdateInProgress:
		return http.StatusPreconditionFailed
	default:
		return http.StatusInternalServerError
	}
}
