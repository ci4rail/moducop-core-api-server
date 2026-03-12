package io4edgemanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/io4edgeartifact"
	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
	"github.com/ci4rail/moducop-core-api-server/pkg/diskstate"
)

const (
	inboxSize             = 10
	updateFirmwareTimeout = 10 * time.Minute
)

type persistentState struct {
	Devices map[string]*io4edgeDevice // key: device name, value: device state
}

type Io4edgeManager struct {
	logger *loglite.Logger
	inbox  chan Command
	quit   chan struct{}
	store  *diskstate.Store[persistentState]
	state  persistentState
}

func New(persistentPath string, logLevel loglite.Level) (*Io4edgeManager, error) {
	m := &Io4edgeManager{
		logger: loglite.New("io4edgemanager", os.Stdout, logLevel),
		inbox:  make(chan Command, inboxSize),
		quit:   make(chan struct{}),
		store:  diskstate.New[persistentState](persistentPath),
	}
	if err := m.store.Load(context.Background(), &m.state); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to load persistent state: %w", err)
		}
		m.state = persistentState{
			Devices: make(map[string]*io4edgeDevice),
		}
		m.logger.Infof("Persistent state file does not exist, initializing new state")
	} else {
		m.logger.Infof("Loaded persistent state: %+v", m.state)
	}
	m.initDeviceStates()
	m.saveState()
	go m.loop()
	return m, nil
}

func (m *Io4edgeManager) loop() {
	for {
		select {
		case <-m.quit:
			m.saveState()
			return
		case cmd := <-m.inbox:
			m.handleCommand(cmd)
		}
		m.saveState()
	}
}

func (m *Io4edgeManager) handleCommand(cmd Command) {
	switch c := cmd.(type) {
	case StartUpdate:
		m.handleUpdate(c.DeviceName, c.PathToFWPKG, c.Reply)
	case GetState:
		m.handleGetState(c.DeviceName, c.Reply)
	case ListDeviceNames:
		m.handleListDeviceNames(c.Reply)
	case cliEvent:
		m.handleCliEvent(c)
	default:
		m.logger.Warnf("Received unknown command: %T", cmd)
	}
}

// handleCliEvent is triggered when io4edge-cli finished. Currently only for device updates
func (m *Io4edgeManager) handleCliEvent(event cliEvent) {
	d, ok := m.state.Devices[event.DeviceName]
	if !ok {
		m.logger.Errorf("device %s not found in state", event.DeviceName)
		return
	}
	m.logger.Infof("Handling CLI event (update done on %s): %v", d.Name, event)
	if event.Success {
		// read back current firmware version
		d.CurrentNV = m.firmwareVersionFromDevice(d)
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
	dev, exists := m.state.Devices[deviceName]
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
	dev, exists := m.state.Devices[deviceName]
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
	devs := make([]string, 0, len(m.state.Devices))
	for name := range m.state.Devices {
		devs = append(devs, name)
	}
	reply <- Result[[]string]{Value: devs}
}

func (m *Io4edgeManager) saveState() {
	err := m.store.Save(context.Background(), m.state)
	if err != nil {
		m.logger.Errorf("Failed to save state: %v", err)
	}
}
