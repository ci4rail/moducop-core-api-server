/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cpumanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/bootid"
	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
	"github.com/ci4rail/moducop-core-api-server/pkg/diskstate"
)

const (
	coreOSEntity  = "coreos"
	inboxSize     = 10
	updateTimeout = 10 * time.Minute
)

type persistentState struct {
	MenderState menderPersistentState
	Entities    map[string]*entity // key: entity name, value: entity
	BootID      string
}

type CPUManager struct {
	logger *loglite.Logger
	inbox  chan Command
	quit   chan struct{}
	store  *diskstate.Store[persistentState]
	state  persistentState
	mender *menderManager
}

func New(persistentPath string, logLevel loglite.Level) (*CPUManager, error) {
	m := &CPUManager{
		logger: loglite.New("cpumanager", os.Stdout, logLevel),
		inbox:  make(chan Command, inboxSize),
		quit:   make(chan struct{}),
		store:  diskstate.New[persistentState](persistentPath),
	}
	hasRebooted := false
	if err := m.store.Load(context.Background(), &m.state); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to load persistent state: %w", err)
		}
		m.setInitialPersistentState()
		m.logger.Infof("Persistent state file does not exist, initializing new state")
	} else {
		m.logger.Infof("Loaded persistent state: %+v", m.state)
		if m.hasRebooted() {
			hasRebooted = true
		}
	}
	m.saveState()
	m.mender = newMenderManager(m.logger, &m.state.MenderState, m.emitMenderEvent, hasRebooted, m.saveState)
	go m.loop()
	return m, nil
}

func (m *CPUManager) loop() {
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

func (m *CPUManager) handleCommand(cmd Command) {
	switch c := cmd.(type) {
	case StartCoreOsUpdate:
		m.handleEntityUpdate(coreOSEntity, entityTypeCoreOs, c.PathToMenderFile, c.Reply)
	case GetCoreOsState:
		m.handleGetEntityState(coreOSEntity, c.Reply)
	case StartApplicationUpdate:
		m.handleEntityUpdate(c.AppName, entityTypeApplication, c.PathToMenderFile, c.Reply)
	case GetApplicationState:
		m.handleGetEntityState(c.AppName, c.Reply)
	case ListApplications:
		m.handleListApplications(c.Reply)
	case Reboot:
		m.handleReboot(c.Reply)
	case MenderEvent:
		m.handleMenderEvent(c)
	default:
		m.logger.Warnf("Received unknown command: %T", cmd)
	}
}

// handleMenderEvent handles events emitted by the mender manager.
func (m *CPUManager) handleMenderEvent(cmd MenderEvent) {
	m.logger.Debugf("Handling mender event: %s", cmd.event.Code)

	switch cmd.event.Code {
	case menderEventNone:
		m.logger.Debugf("Ignoring no-op mender event")
	case menderEventInstallFinished, menderEventRebootFinished, menderEventCommitFinished, menderEventRestarted, menderEventRecoverFinished:
		// pass low level events to mender manager
		m.mender.HandleEvent(cmd.event)
	case menderEventJobFinished:
		m.logger.Infof("Mender job finished with success=%v, message=%s", cmd.event.Success, cmd.event.Message)
		m.handleMenderJobFinished(cmd.event.Success, cmd.event.Message)
	default:
		m.logger.Warnf("Received unknown mender event code: %d", cmd.event.Code)
	}
}

// emitMenderEvent sends a mender event to the manager's inbox.
// This is used by the mender manager to notify about events.
func (m *CPUManager) emitMenderEvent(event menderEvent) {
	m.inbox <- MenderEvent{event: event}
}

func (m *CPUManager) handleEntityUpdate(
	entityName string,
	entityType entityType,
	menderArtifact string,
	reply chan Result[struct{}],
) {
	if m.rejectInvalidCoreOSEntity(entityName, entityType, reply) {
		return
	}

	e, ok := m.state.Entities[entityName]
	if !ok {
		m.state.Entities[entityName] = newEntity(entityName, entityType)
		e = m.state.Entities[entityName]
	}
	if m.rejectUpdateInProgress(entityName, e, reply) {
		return
	}

	deployingNV, err := e.getVersionFromArtifact(menderArtifact)
	if err != nil {
		m.logger.Errorf("Failed to get version from artifact for entity %s: %v", entityName, err)
		reply <- Result[struct{}]{Err: NewCodedError(
			ErrCodeArtifactInvalid,
			fmt.Sprintf("invalid artifact for entity %s: %v", entityName, err),
		)}
		return
	}

	if m.rejectAlreadyDeployed(entityName, e, deployingNV, reply) {
		return
	}

	e.MenderArtifact = menderArtifact
	e.DeployingNV = deployingNV

	if !m.mender.IsIdle() {
		m.logger.Infof("Received update command for entity %s, but mender is currently busy with another update", entityName)
		e.DeployStatus.Code = DeployStatusCodeWaiting
		e.DeployStatus.Message = "Mender is busy with another update"
		reply <- Result[struct{}]{}
		return
	}
	err = m.startEntityUpdate(e)
	if err != nil {
		m.logger.Error(err)
		reply <- Result[struct{}]{Err: err}
		return
	}

	reply <- Result[struct{}]{}
}

func (m *CPUManager) rejectInvalidCoreOSEntity(entityName string, entityType entityType, reply chan Result[struct{}]) bool {
	if entityType != entityTypeCoreOs || entityName == coreOSEntity {
		return false
	}
	reply <- Result[struct{}]{Err: NewCodedError(
		ErrCodeInvalidCoreOSEntityName,
		fmt.Sprintf("invalid entity name for CoreOS: %s", entityName),
	)}
	return true
}

func (m *CPUManager) rejectUpdateInProgress(entityName string, e *entity, reply chan Result[struct{}]) bool {
	if e.DeployStatus.Code != DeployStatusCodeInProgress && e.DeployStatus.Code != DeployStatusCodeWaiting {
		return false
	}
	m.logger.Infof("Received update command for entity %s, but an update is already in progress or waiting", entityName)
	reply <- Result[struct{}]{Err: NewCodedError(
		ErrCodeEntityUpdateInProgress,
		fmt.Sprintf("an update is already in progress or waiting for entity %s", entityName),
	)}
	return true
}

func (m *CPUManager) rejectAlreadyDeployed(entityName string, e *entity, deployingNV NameVersion, reply chan Result[struct{}]) bool {
	deployed, err := e.isDeployed(deployingNV)
	if err != nil {
		m.logger.Warnf("Failed to check if artifact is already deployed for entity %s: %v. Continue", entityName, err)
		return false
	}
	if !deployed {
		return false
	}
	m.logger.Infof("Received update command for entity %s, but the same version is already deployed", entityName)
	reply <- Result[struct{}]{Err: NewCodedError(
		ErrCodeAlreadyDeployed,
		fmt.Sprintf("the same version is already deployed for entity %s", entityName),
	)}
	return true
}

func (m *CPUManager) handleGetEntityState(entityName string, reply chan Result[EntityStatus]) {
	e, ok := m.state.Entities[entityName]
	if !ok {
		reply <- Result[EntityStatus]{Err: NewCodedError(
			ErrCodeEntityNotFound,
			fmt.Sprintf("no such entity: %s", entityName),
		)}
		return
	}
	nv, err := e.getDeployedVersion()
	if err != nil {
		m.logger.Warnf("Failed to get deployed version for entity %s: %v. Assume not deployed", entityName, err)
		nv = NameVersion{}
	}
	// if err != nil && e.DeployStatus.Code == DeployStatusCodeSuccess {
	// 	m.logger.Errorf("Failed to get deployed version for entity %s: %v. Assume ", entityName, err)
	// 	reply <- Result[EntityStatus]{Err: NewCodedError(ErrCodeGetVersionFailed, fmt.Sprintf("failed to get deployed version for entity %s: %v", entityName, err))
	// 		fmt.Errorf("failed to get deployed version for entity %s: %w", entityName, err)}
	// 	return
	// }
	reply <- Result[EntityStatus]{Value: EntityStatus{
		DeployStatus: e.DeployStatus,
		Current: NameVersion{
			nv.Name,
			nv.Version,
		},
	}}
}

func (m *CPUManager) handleListApplications(reply chan Result[[]string]) {
	apps, err := listApplicationsFromTargetFS()
	if err != nil {
		m.logger.Errorf("Failed to list applications: %v", err)
		reply <- Result[[]string]{Err: NewCodedError(
			ErrCodeListApplicationsFailed,
			fmt.Sprintf("failed to list applications: %v", err),
		)}
		return
	}
	reply <- Result[[]string]{Value: apps}
}

func (m *CPUManager) handleReboot(reply chan Result[struct{}]) {
	m.logger.Infof("Rebooting system as requested by API")
	m.mender.runRebootInBackGround(rebootTimeout)
	reply <- Result[struct{}]{}
}

func (m *CPUManager) handleMenderJobFinished(success bool, message string) {
	e := m.getEntityInProgress()
	if e == nil {
		m.logger.Warnf("Mender job finished with success=%v, message=%s, but no entity was marked as in progress", success, message)
	} else {
		// finish current update
		m.finishEntityUpdate(e, success, message)
	}
	// start next waiting update
	waiting := m.getEntityWaiting()
	if waiting != nil {
		m.logger.Infof("Starting next update for entity %s that was waiting", waiting.Name)
		err := m.startEntityUpdate(waiting)
		if err != nil {
			m.logger.Error(err)
		}
	} else {
		m.logger.Infof("No waiting updates, actor is now idle")
	}
}

func (m *CPUManager) hasRebooted() bool {
	currentBootID, err := bootid.Get()
	if err != nil {
		m.logger.Errorf("Failed to read current boot ID: %v", err)
		return false
	}
	if m.state.BootID != currentBootID {
		m.logger.Infof("Detected reboot with new boot ID: %s (previous: %s)", currentBootID, m.state.BootID)
		m.state.BootID = currentBootID
		return true
	}
	return false
}

func (m *CPUManager) setInitialPersistentState() {
	m.state = persistentState{
		Entities: make(map[string]*entity),
	}
	m.state.Entities[coreOSEntity] = newEntity(coreOSEntity, entityTypeCoreOs)
	m.state.MenderState = menderPersistentState{
		State:           menderStateIdle,
		CurrentArtifact: "",
	}
	bootid, err := bootid.Get()
	if err != nil {
		m.logger.Errorf("Failed to read boot ID during initial state setup: %v", err)
		m.state.BootID = ""
	} else {
		m.state.BootID = bootid
	}
	m.logger.Debugf("Initialized state with %+v", m.state)
}

func (m *CPUManager) saveState() {
	m.logger.Debugf("save state: %+v", m.state)
	err := m.store.Save(context.Background(), m.state)
	if err != nil {
		m.logger.Errorf("Failed to save state: %v", err)
	}
}
