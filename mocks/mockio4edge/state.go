package mockio4edge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultStateDir     = "/tmp/mock-mender"
	stateFileName       = "io4edge-state.json"
	defaultFirmwareName = "fw-iou16-00-default"
	defaultVersion      = "1.0.0"
	devicePrefix        = "S100-IUO16-USB-EXT1"
)

var deviceIDs = []string{
	devicePrefix + "-UC1",
	devicePrefix + "-UC2",
	devicePrefix + "-UC3",
	devicePrefix + "-UC4",
	devicePrefix + "-UC5",
}

var serialByDevice = map[string]string{
	devicePrefix + "-UC1": "b4e31793-f660-4e2e-af20-c175186b95be",
	devicePrefix + "-UC2": "1d69c4b7-22a0-4d1f-a2f4-8dba8a7f7c91",
	devicePrefix + "-UC3": "8e3a6cf7-8d8f-4720-9309-bcbf7aeff190",
	devicePrefix + "-UC4": "64087834-42ad-4db5-87dc-dca621f0814e",
	devicePrefix + "-UC5": "3d4dc019-f95f-4f9f-bd33-5999c99ff7af",
}

type State struct {
	FirmwareByDevice map[string]string `json:"firmware_by_device"`
}

func DeviceIDs() []string {
	ids := make([]string, 0, len(deviceIDs))
	ids = append(ids, deviceIDs...)
	return ids
}

func FirmwareName() string {
	return defaultFirmwareName
}

func StateDir() string {
	if v := os.Getenv("MOCK_MENDER_STATE_DIR"); v != "" {
		return v
	}
	return defaultStateDir
}

func StatePath() string {
	return filepath.Join(StateDir(), stateFileName)
}

func LoadState() (State, error) {
	p := StatePath()
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s := defaultState()
			if saveErr := SaveState(s); saveErr != nil {
				return State{}, saveErr
			}
			return s, nil
		}
		return State{}, err
	}

	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	normalize(&s)
	return s, nil
}

func SaveState(s State) error {
	if err := os.MkdirAll(StateDir(), 0o755); err != nil {
		return err
	}
	normalize(&s)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StatePath(), b, 0o600)
}

func ResolveDeviceID(input string) (string, bool) {
	for _, id := range deviceIDs {
		if id == input {
			return id, true
		}
	}
	if input == devicePrefix {
		return devicePrefix + "-UC1", true
	}
	var match string
	for _, id := range deviceIDs {
		if strings.HasPrefix(id, input) {
			if match != "" {
				return "", false
			}
			match = id
		}
	}
	if match == "" {
		return "", false
	}
	return match, true
}

func DeviceSerial(deviceID string) (string, bool) {
	serial, ok := serialByDevice[deviceID]
	return serial, ok
}

func DeviceIP(deviceID string) (string, bool) {
	for i, id := range deviceIDs {
		if id == deviceID {
			return fmt.Sprintf("192.168.%d.1", 200+i), true
		}
	}
	return "", false
}

func defaultState() State {
	fw := make(map[string]string, len(deviceIDs))
	for _, id := range deviceIDs {
		fw[id] = defaultVersion
	}
	return State{FirmwareByDevice: fw}
}

func normalize(s *State) {
	if s.FirmwareByDevice == nil {
		s.FirmwareByDevice = make(map[string]string, len(deviceIDs))
	}
	for _, id := range deviceIDs {
		if s.FirmwareByDevice[id] == "" {
			s.FirmwareByDevice[id] = defaultVersion
		}
	}
}
