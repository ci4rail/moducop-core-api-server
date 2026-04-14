/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package io4edgemanager

import "fmt"

type io4edgeDevice struct {
	Name         string // device name as reported by io4edge-cli
	DeployStatus DeployStatus
	FwPackage    string      // "" if no update is in progress
	DeployingNV  NameVersion // the version that is being deployed, empty if no deployment in progress
	CurrentNV    NameVersion // the currently installed firmware version
}

func (m *Io4edgeManager) scanAndUpdateDeviceStates() {
	// get list of devices
	devices, err := m.scanDevices()
	if err != nil {
		m.logger.Errorf("Failed to scan devices: %v", err)
		return
	}

	m.removeNoLongerPresentDevices(devices)

	// add new devices to state
	for _, device := range devices {
		if err := m.updateDeviceState(device); err != nil {
			m.logger.Errorf("Failed to update device state for device %s: %v", device, err)
		}
	}
}

// updateDeviceState updates the state of a single device by reading its current firmware version.
// If the device is not already in the state, it will be added.
// If the device is no longer present, it will be removed from the state.
func (m *Io4edgeManager) updateDeviceState(deviceName string) error {
	currentNV := m.firmwareVersionFromDevice(deviceName)
	if currentNV.Name == "" && currentNV.Version == "" {
		// remove device from state
		delete(m.deviceState, deviceName)
		return fmt.Errorf("failed to get firmware version for device %s", deviceName)
	}

	dev, ok := m.deviceState[deviceName]
	if !ok {
		dev := &io4edgeDevice{
			Name: deviceName,
		}
		dev.DeployStatus.Code = DeployStatusCodeNeverDeployed
		m.deviceState[deviceName] = dev
	}
	dev.CurrentNV = currentNV
	return nil
}

// remove entries from state that are no longer present
func (m *Io4edgeManager) removeNoLongerPresentDevices(devices []string) {

	for d := range m.deviceState {
		found := false
		for _, dev := range devices {
			if dev == d {
				found = true
				break
			}
		}
		if !found {
			m.logger.Infof("Removing device %s from state since it is no longer present", d)
			delete(m.deviceState, d)
		}
	}
}
