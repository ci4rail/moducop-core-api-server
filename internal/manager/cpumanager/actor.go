package cpumanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
	"github.com/ci4rail/moducop-core-api-server/pkg/diskstate"
)

const (
	coreOSEntity       = "coreos"
	inboxSize          = 10
	updateStartTimeout = 10 * time.Minute
)

type entityType int

const (
	entityTypeCoreOs entityType = iota
	entityTypeApplication
)

type entity struct {
	Name         string
	EntityType   entityType
	DeployStatus DeployStatus
	MenderFile   string // "" if no update is in progress
}

type persistenState struct {
	MenderState menderPersistentState
	Entities    map[string]*entity // key: entity name, value: entity
}

type CPUManager struct {
	logger *loglite.Logger
	inbox  chan command
	quit   chan struct{}
	store  *diskstate.Store[persistenState]
	state  persistenState
	mender *menderManager
}

func New(persistentPath string, logLevel loglite.Level) (*CPUManager, error) {
	m := &CPUManager{
		logger: loglite.New("cpumanager", os.Stdout, logLevel),
		inbox:  make(chan command, inboxSize),
		quit:   make(chan struct{}),
		store:  diskstate.New[persistenState](persistentPath),
	}

	if err := m.store.Load(context.Background(), &m.state); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to load persistent state: %w", err)
		}
		m.logger.Infof("Persistent state file does not exist, initializing new state")
		m.state = persistenState{
			Entities: make(map[string]*entity),
		}
		m.state.Entities[coreOSEntity] = &entity{
			Name:         coreOSEntity,
			EntityType:   entityTypeCoreOs,
			DeployStatus: DeployStatus{Code: DeployStatusCodeNeverDeployed, Message: "CoreOS has never been deployed"},
		}
		m.state.MenderState = menderPersistentState{
			State:       menderStateIdle,
			CurrentFile: "",
		}
		m.logger.Infof("Initialized state with %+v", m.state)
		if err := m.saveState(); err != nil {
			return nil, err
		}
	}

	m.mender = newMenderManager(m.logger, &m.state.MenderState, m.emitMenderEvent)
	go m.loop()
	return m, nil
}

func (m *CPUManager) saveState() error {
	err := m.store.Save(context.Background(), m.state)
	if err != nil {
		m.logger.Errorf("Failed to save state: %v", err)
	}
	return err
}

func (m *CPUManager) loop() {
	for {
		select {
		case <-m.quit:
			return
		case cmd := <-m.inbox:
			m.handleCommand(cmd)
		}
		if err := m.saveState(); err != nil {
			m.logger.Errorf("Failed to persist state after command: %v", err)
		}
	}
}

func (m *CPUManager) handleCommand(cmd command) {
	switch c := cmd.(type) {
	case StartCoreOsUpdate:
		m.handleEntityUpdate(coreOSEntity, entityTypeCoreOs, c.PathToMenderFile, c.Reply)
	case GetCoreOsState:
		m.logger.Debugf("GetCoreOsState not implemented yet: %+v", c)
	case StartApplicationUpdate:
		m.handleEntityUpdate(c.AppName, entityTypeApplication, c.PathToMenderFile, c.Reply)
	case GetApplicationState:
		m.logger.Debugf("GetApplicationState not implemented yet: %+v", c)
	case Reboot:
		m.logger.Debugf("Reboot not implemented yet: %+v", c)
	case MenderEvent:
		m.handleMenderEvent(c)
	default:
		m.logger.Warnf("Received unknown command: %T", cmd)
	}
}

func (m *CPUManager) handleMenderEvent(cmd MenderEvent) {
	m.logger.Infof("Handling mender event: %d", cmd.event.Code)

	switch cmd.event.Code {
	case menderEventNone:
		m.logger.Debugf("Ignoring no-op mender event")
	case menderEventInstallFinished, menderEventRebootFinished, menderEventCommitFinished:
		// pass low level events to mender manager
		m.mender.HandleEvent(cmd.event)
	case menderEventJobFinished:
		m.logger.Infof("Mender job finished with success=%v, message=%s", cmd.event.Success, cmd.event.Message)
		m.handleMenderJobFinished(cmd.event.Success, cmd.event.Message)
	default:
		m.logger.Warnf("Received unknown mender event code: %d", cmd.event.Code)
	}
}

func (m *CPUManager) emitMenderEvent(event menderEvent) {
	m.inbox <- MenderEvent{event: event}
}

func (m *CPUManager) handleEntityUpdate(
	entityName string,
	entityType entityType,
	menderFile string,
	reply chan Result[struct{}],
) {
	if entityType == entityTypeCoreOs && entityName != coreOSEntity {
		reply <- Result[struct{}]{Err: NewCodedError(
			ErrCodeInvalidCoreOSEntityName,
			fmt.Sprintf("invalid entity name for CoreOS: %s", entityName),
		)}
		return
	}

	e, ok := m.state.Entities[entityName]
	if !ok {
		m.state.Entities[entityName] = &entity{
			Name:         entityName,
			EntityType:   entityType,
			DeployStatus: DeployStatus{Code: DeployStatusCodeNeverDeployed, Message: "Entity has never been deployed"},
			MenderFile:   "",
		}
		e = m.state.Entities[entityName]
	}
	if e.DeployStatus.Code == DeployStatusCodeInProgress || e.DeployStatus.Code == DeployStatusCodeWaiting {
		m.logger.Infof("Received update command for entity %s, but an update is already in progress or waiting", entityName)
		reply <- Result[struct{}]{Err: NewCodedError(
			ErrCodeEntityUpdateInProgress,
			fmt.Sprintf("an update is already in progress or waiting for entity %s", entityName),
		)}
		return
	}

	e.MenderFile = menderFile

	if !m.mender.IsIdle() {
		m.logger.Infof("Received update command for entity %s, but mender is currently busy with another update", entityName)
		e.DeployStatus.Code = DeployStatusCodeWaiting
		reply <- Result[struct{}]{}
		return
	}
	err := m.startEntityUpdate(e)
	if err != nil {
		m.logger.Error(err)
		reply <- Result[struct{}]{Err: err}
		return
	}

	reply <- Result[struct{}]{}
}

func (m *CPUManager) startEntityUpdate(e *entity) error {
	e.DeployStatus.Code = DeployStatusCodeInProgress
	err := m.mender.StartUpdateJob(e.EntityType, e.MenderFile, updateStartTimeout)
	if err != nil {
		e.DeployStatus.Code = DeployStatusCodeFailure
		e.DeployStatus.Message = fmt.Sprintf("Failed to start update: %v", err)
		if errors.Is(err, ErrMenderBusy) {
			return WrapCodedError(
				ErrCodeMenderBusy,
				fmt.Sprintf("mender is busy, cannot start update for entity %s", e.Name),
				err,
			)
		}
		return WrapCodedError(
			ErrCodeStartUpdateFailed,
			fmt.Sprintf("failed to start mender update job for entity %s", e.Name),
			err,
		)
	}
	return nil
}

func (m *CPUManager) handleMenderJobFinished(success bool, message string) {
	e := m.getEntityInProgress()
	if e == nil {
		m.logger.Warnf("Mender job finished with success=%v, message=%s, but no entity was marked as in progress", success, message)
		return
	}
	// finish current update
	m.finishEntityUpdate(e, success, message)
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

func (m *CPUManager) getEntityInProgress() *entity {
	for _, e := range m.state.Entities {
		if e.DeployStatus.Code == DeployStatusCodeInProgress {
			return e
		}
	}
	return nil
}

func (m *CPUManager) getEntityWaiting() *entity {
	for _, e := range m.state.Entities {
		if e.DeployStatus.Code == DeployStatusCodeWaiting {
			return e
		}
	}
	return nil
}

func (m *CPUManager) finishEntityUpdate(e *entity, success bool, message string) {
	if success {
		e.DeployStatus.Code = DeployStatusCodeSuccess
		e.DeployStatus.Message = "Update deployed successfully"
	} else {
		e.DeployStatus.Code = DeployStatusCodeFailure
		e.DeployStatus.Message = message
	}
	e.MenderFile = ""
}
