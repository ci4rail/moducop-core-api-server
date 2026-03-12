package io4edgemanager

import (
	"fmt"
	"strings"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/execcli"
)

const (
	scanTimeout      = 10 * time.Second
	getFwTimeout     = 5 * time.Second
	minDeviceColumns = 4
)

func (m *Io4edgeManager) startUpdate(deviceName string) {
	d, ok := m.state.Devices[deviceName] 
	if !ok {
		m.logger.Errorf("device %s not found in state", deviceName)
		return
	}
	go func() {
		stdout, stderr, err := m.runIo4edgeCLI(updateFirmwareTimeout, "-d", deviceName, "load-firmware", d.FwPackage)
		ce := cliEvent{
			DeviceName: deviceName,
			Success: err == nil,
			Message: fmt.Sprintf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err),
		}
		m.logger.Infof("io4edge-cli finished: %v", ce)
		m.inbox <- ce
	}()
}



func (m *Io4edgeManager) runIo4edgeCLI(timeout time.Duration, args ...string) (string, string, error) {
	stdout, stderr, _, err := execcli.RunCommand("io4edge-cli", timeout, args...)
	return stdout, stderr, err
}

// scanDevices calls io4edge-cli scan to get list of devices
func (m *Io4edgeManager) scanDevices() ([]string, error) {
	stdout, stderr, err := m.runIo4edgeCLI(scanTimeout, "scan")
	if err != nil {
		m.logger.Errorf("io4edge-cli scan failed: %v, stderr: %s", err, stderr)
		return nil, err
	}
	// Parse stdout to extract device list
	devices := parseDevices(stdout)
	return devices, nil
}

func parseDevices(output string) []string {
	lines := strings.Split(output, "\n")
	devices := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "DEVICE ID") {
			continue
		}

		fields := strings.Fields(line)
		// Expected columns: DEVICE ID, IP, HARDWARE, SERIAL.
		if len(fields) < minDeviceColumns {
			continue
		}

		// First column is DEVICE ID.
		devices = append(devices, fields[0])
	}

	return devices
}

func (m *Io4edgeManager) firmwareVersionFromDevice(d *io4edgeDevice) NameVersion {
	stdout, stderr, err := m.runIo4edgeCLI(getFwTimeout, "-d", d.Name, "fw")
	if err != nil {
		m.logger.Errorf("io4edge-cli fw failed: %v, stderr: %s", err, stderr)
		return NameVersion{}
	}
	// Parse stdout to extract firmware version
	return parseFirmwareVersion(stdout)
}

func parseFirmwareVersion(output string) NameVersion {
	const prefix = "Firmware name:"

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		namePart, versionPart, ok := strings.Cut(rest, ",")
		if !ok {
			return NameVersion{}
		}

		name := strings.TrimSpace(namePart)
		versionPart = strings.TrimSpace(versionPart)
		versionPart = strings.TrimPrefix(versionPart, "Version")
		version := strings.TrimSpace(versionPart)
		if name == "" || version == "" {
			return NameVersion{}
		}

		return NameVersion{
			Name:    name,
			Version: version,
		}
	}

	return NameVersion{}
}
