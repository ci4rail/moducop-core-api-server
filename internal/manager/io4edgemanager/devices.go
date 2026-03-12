package io4edgemanager

type io4edgeDevice struct {
	Name         string
	DeployStatus DeployStatus
	FwPackage    string      // "" if no update is in progress
	DeployingNV  NameVersion // the version that is being deployed, empty if no deployment in progress
	CurrentNV    NameVersion // the currently installed firmware version
}

func (m *Io4edgeManager) getDeviceInProgress() *io4edgeDevice {
	for _, d := range m.state.Devices {
		if d.DeployStatus.Code == DeployStatusCodeInProgress {
			return d
		}
	}
	return nil
}

func (m *Io4edgeManager) startUpdate(deviceName string) {
	m.runIo4edgeCLIInBackGround(updateFirmwareTimeout, "-d", deviceName, "load-firmware", m.state.Devices[deviceName].FwPackage)
}

func (m *Io4edgeManager) initDeviceStates() {
	// if there was an update in progress, restart it
	if d := m.getDeviceInProgress(); d != nil {
		m.logger.Infof("Resuming in-progress update for device %s", d.Name)
		m.startUpdate(d.Name)
	}

	// get list of devices
	devices, err := m.scanDevices()
	if err != nil {
		m.logger.Errorf("Failed to scan devices: %v", err)
		return
	}
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
