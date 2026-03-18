/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package io4edgemanager

type io4edgeDevice struct {
	Name         string
	DeployStatus DeployStatus
	FwPackage    string      // "" if no update is in progress
	DeployingNV  NameVersion // the version that is being deployed, empty if no deployment in progress
	CurrentNV    NameVersion // the currently installed firmware version
}

func (m *Io4edgeManager) initDeviceStates() {
	// get list of devices
	devices, err := m.scanDevices()
	if err != nil {
		m.logger.Errorf("Failed to scan devices: %v", err)
		return
	}

	m.removeNoLongerPresentDevices(devices)

	// if there was an update in progress, restart it
	for d := range m.state.Devices {
		if m.state.Devices[d].DeployStatus.Code == DeployStatusCodeInProgress {
			m.logger.Infof("Resuming in-progress update for device %s", d)
			m.startUpdate(d)
		}
	}

	// add new devices to state
	for _, device := range devices {
		if _, exists := m.state.Devices[device]; !exists {
			d := &io4edgeDevice{
				Name: device,
			}
			// device not known, get current firmware version
			d.CurrentNV = m.firmwareVersionFromDevice(d)
			d.DeployStatus.Code = DeployStatusCodeNeverDeployed
			m.state.Devices[device] = d
		}
	}
}

// remove entries from state that are no longer present
func (m *Io4edgeManager) removeNoLongerPresentDevices(devices []string) {

	for d := range m.state.Devices {
		found := false
		for _, dev := range devices {
			if dev == d {
				found = true
				break
			}
		}
		if !found {
			m.logger.Infof("Removing device %s from state since it is no longer present", d)
			delete(m.state.Devices, d)
		}
	}
}
