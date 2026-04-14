/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package io4edgemanager

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/io4edgeartifact"
	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
)

const (
	inboxSize             = 10
	updateFirmwareTimeout = 10 * time.Minute
)

type Io4edgeManager struct {
	logger      *loglite.Logger
	inbox       chan Command
	quit        chan struct{}
	deviceState map[string]*io4edgeDevice // key: device name, value: device state
}

func New(persistentPath string, logLevel loglite.Level) (*Io4edgeManager, error) {
	m := &Io4edgeManager{
		logger: loglite.New("io4edgemanager", os.Stdout, logLevel),
		inbox:  make(chan Command, inboxSize),
		quit:   make(chan struct{}),
	}
	m.deviceState = make(map[string]*io4edgeDevice)
	go m.loop()
	return m, nil
}

func (m *Io4edgeManager) loop() {
	for {
		select {
		case <-m.quit:
			return
		case cmd := <-m.inbox:
			m.handleCommand(cmd)
		}
	}
}

func (m *Io4edgeManager) handleCommand(cmd Command) {
	switch c := cmd.(type) {
	case StartUpdate:
		_ = m.updateDeviceState(c.DeviceName)
		m.handleUpdate(c.DeviceName, c.PathToFWPKG, c.Reply)
	case GetState:
		_ = m.updateDeviceState(c.DeviceName)
		m.handleGetState(c.DeviceName, c.Reply)
	case ListDeviceNames:
		m.scanAndUpdateDeviceStates()
		m.handleListDeviceNames(c.Reply)
	case cliEvent:
		m.handleCliEvent(c)
	default:
		m.logger.Warnf("Received unknown command: %T", cmd)
	}
}

// handleCliEvent is triggered when io4edge-cli finished. Currently only for device updates
func (m *Io4edgeManager) handleCliEvent(event cliEvent) {
	d, ok := m.deviceState[event.DeviceName]
	if !ok {
		m.logger.Errorf("device %s not found in state", event.DeviceName)
		return
	}
	m.logger.Infof("Handling CLI event (update done on %s): %v", d.Name, event)
	if event.Success {
		// read back current firmware version
		d.CurrentNV = m.firmwareVersionFromDevice(d.Name)
		if d.CurrentNV.Name == d.DeployingNV.Name && d.CurrentNV.Version == d.DeployingNV.Version {
			d.DeployStatus = DeployStatus{
				Code:    DeployStatusCodeSuccess,
				Message: "Update successful",
			}
		} else {
			d.DeployStatus = DeployStatus{
				Code:    DeployStatusCodeFailure,
				Message: fmt.Sprintf("Firmware version after update does not match expected version. Current: %s %s, Expected: %s %s", d.CurrentNV.Name, d.CurrentNV.Version, d.DeployingNV.Name, d.DeployingNV.Version),
			}
		}
	} else {
		d.DeployStatus = DeployStatus{
			Code:    DeployStatusCodeFailure,
			Message: fmt.Sprintf("Update failed: %s", event.Message),
		}
	}
	d.FwPackage = ""
	d.DeployingNV = NameVersion{}
}

func (m *Io4edgeManager) handleUpdate(
	deviceName string,
	fwPackage string,
	reply chan Result[struct{}],
) {
	dev, exists := m.deviceState[deviceName]
	if !exists {
		reply <- Result[struct{}]{Err: NewCodedError(ErrCodeDeviceNotFound, "Unknown device "+deviceName)}
		return
	}
	if dev.DeployStatus.Code == DeployStatusCodeInProgress {
		reply <- Result[struct{}]{Err: NewCodedError(ErrCodeDeviceUpdateInProgress, "Update already in progress for device "+deviceName)}
		return
	}
	manifest, err := io4edgeartifact.GetManifestFromFile(fwPackage)
	if err != nil {
		reply <- Result[struct{}]{Err: NewCodedError(ErrCodeArtifactInvalid, fmt.Sprintf("Failed to read firmware package: %v", err))}
		return
	}
	manifest.Name = tweakNameFromManifest(manifest.Name)
	if manifest.Name == dev.CurrentNV.Name && manifest.Version == dev.CurrentNV.Version {
		reply <- Result[struct{}]{Err: NewCodedError(ErrCodeAlreadyDeployed, "Device "+deviceName+" already has firmware version "+manifest.Version)}
		return
	}
	dev.FwPackage = fwPackage
	dev.DeployingNV = NameVersion{
		Name:    manifest.Name,
		Version: manifest.Version,
	}
	dev.DeployStatus = DeployStatus{
		Code:    DeployStatusCodeInProgress,
		Message: "Update is in progress",
	}
	m.startUpdate(dev.Name)
	reply <- Result[struct{}]{}
}

func (m *Io4edgeManager) handleGetState(deviceName string, reply chan Result[Io4edgeFWStatus]) {
	dev, exists := m.deviceState[deviceName]
	if !exists {
		reply <- Result[Io4edgeFWStatus]{Err: NewCodedError(ErrCodeDeviceNotFound, "Unknown device "+deviceName)}
		return
	}
	reply <- Result[Io4edgeFWStatus]{Value: Io4edgeFWStatus{
		DeployStatus: dev.DeployStatus,
		Current:      dev.CurrentNV,
	}}
}

func (m *Io4edgeManager) handleListDeviceNames(reply chan Result[[]string]) {
	devs := make([]string, 0, len(m.deviceState))
	for name := range m.deviceState {
		devs = append(devs, name)
	}
	// sort device names for consistent order
	sort.Strings(devs)

	reply <- Result[[]string]{Value: devs}
}

// In most manifest files, the name is missing the leading "fw-" part, but firmware reports
// the full name with "fw-" prefix. So we add it here if missing to be able to compare versions.
func tweakNameFromManifest(name string) string {
	if !strings.HasPrefix(name, "fw-") {
		return "fw-" + name
	}
	return name
}
