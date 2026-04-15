/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package mockio4edge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultStateDir     = "/tmp/mock-mender"
	stateFileName       = "io4edge-state.json"
	defaultFirmwareName = "fw-iou16-00-default"
	defaultVersion      = "1.0.0"
	devicePrefix        = "S100-IUO16-USB-EXT1"
	restartDowntime     = 10 * time.Second
	stateLockTimeout    = 5 * time.Second
	stateLockRetry      = 10 * time.Millisecond
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
	FirmwareByDevice         map[string]string    `json:"firmware_by_device"`
	UnavailableUntilByDevice map[string]time.Time `json:"unavailable_until_by_device"`
}

func DeviceIDs() []string {
	ids := make([]string, 0, len(deviceIDs))
	ids = append(ids, deviceIDs...)
	return ids
}

func FirmwareName() string {
	return defaultFirmwareName
}

func RestartDowntime() time.Duration {
	return restartDowntime
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
	return loadStateUnlocked()
}

func UpdateState(update func(*State) error) (State, error) {
	var updated State
	err := withStateLock(func() error {
		s, err := loadStateUnlocked()
		if err != nil {
			return err
		}
		if err := update(&s); err != nil {
			return err
		}
		if err := saveStateUnlocked(s); err != nil {
			return err
		}
		updated = s
		return nil
	})
	if err != nil {
		return State{}, err
	}
	return updated, nil
}

func loadStateUnlocked() (State, error) {
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
	return saveStateUnlocked(s)
}

func saveStateUnlocked(s State) error {
	if err := os.MkdirAll(StateDir(), 0o755); err != nil {
		return err
	}
	normalize(&s)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(StateDir(), stateFileName+".tmp.*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, StatePath()); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func ResolveDeviceID(input string) (string, bool) {
	if deviceID, ok := deviceIDFromIP(strings.TrimSuffix(input, ":9999")); ok {
		return deviceID, true
	}
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
	return State{
		FirmwareByDevice:         fw,
		UnavailableUntilByDevice: make(map[string]time.Time, len(deviceIDs)),
	}
}

func normalize(s *State) {
	if s.FirmwareByDevice == nil {
		s.FirmwareByDevice = make(map[string]string, len(deviceIDs))
	}
	if s.UnavailableUntilByDevice == nil {
		s.UnavailableUntilByDevice = make(map[string]time.Time, len(deviceIDs))
	}
	for _, id := range deviceIDs {
		if s.FirmwareByDevice[id] == "" {
			s.FirmwareByDevice[id] = defaultVersion
		}
	}
	now := time.Now()
	for deviceID, until := range s.UnavailableUntilByDevice {
		if !now.Before(until) {
			delete(s.UnavailableUntilByDevice, deviceID)
		}
	}
}

func (s State) IsAvailable(deviceID string, now time.Time) bool {
	until, ok := s.UnavailableUntilByDevice[deviceID]
	return !ok || !now.Before(until)
}

func deviceIDFromIP(input string) (string, bool) {
	for _, id := range deviceIDs {
		ip, ok := DeviceIP(id)
		if ok && ip == input {
			return id, true
		}
	}
	return "", false
}

func withStateLock(fn func() error) error {
	if err := os.MkdirAll(StateDir(), 0o755); err != nil {
		return err
	}
	lockPath := StatePath() + ".lock"
	deadline := time.Now().Add(stateLockTimeout)
	for {
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = lockFile.Close()
			defer func() {
				_ = os.Remove(lockPath)
			}()
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout acquiring state lock: %s", lockPath)
		}
		time.Sleep(stateLockRetry)
	}
}
