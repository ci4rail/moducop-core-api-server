package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/execcli"
)

const (
	// TODO: This is not generic.
	eepromSpec   = "/sys/bus/i2c/devices/3-0050/eeprom@256:256"
	eeInvTimeout = 5 * time.Second
)

type hardwareInfo struct {
	Vendor       string `json:"vendor"`
	Model        string `json:"model"`
	Variant      int    `json:"variant"`
	MajorVersion int    `json:"majorVersion"`
	Serial       string `json:"serial"`
}

func parseHardwareInfo(stdout string) (hardwareInfo, error) {
	var info hardwareInfo
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		return hardwareInfo{}, err
	}
	return info, nil
}

func (a *API) handleGetHardwareInfo(w http.ResponseWriter, _ *http.Request) {
	stdout, stderr, _, err := execcli.RunCommand("ee-inv", eeInvTimeout, eepromSpec)
	if err != nil {
		a.logger.Errorf("failed to execute ee-inv: %v, stderr: %s", err, stderr)
		a.writeJSONError(w, http.StatusInternalServerError, errCodeEeInvFailed, fmt.Sprintf("failed to get hardware info: %v", err))
		return
	}
	info, err := parseHardwareInfo(stdout)
	if err != nil {
		a.logger.Errorf("failed to parse ee-inv output: %v, output: %s", err, stdout)
		a.writeJSONError(w, http.StatusInternalServerError, errCodeEeInvFailed, fmt.Sprintf("failed to parse hardware info: %v", err))
		return
	}
	a.writeJSON(w, info)
}
