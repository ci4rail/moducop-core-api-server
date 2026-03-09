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
		m.state.Entities[coreOSEntity] = newEntity(coreOSEntity, entityTypeCoreOs)
		m.state.MenderState = menderPersistentState{
			State:           menderStateIdle,
			CurrentArtifact: "",
		}
		m.logger.Infof("Initialized state with %+v", m.state)
		m.saveState()
	}

	m.mender = newMenderManager(m.logger, &m.state.MenderState, m.emitMenderEvent)
	go m.loop()
	return m, nil
}

func (m *CPUManager) saveState()  {
	err := m.store.Save(context.Background(), m.state)
	if err != nil {
		m.logger.Errorf("Failed to save state: %v", err)
	}
}

func (m *CPUManager) loop() {
	for {
		select {
		case <-m.quit:
			m.saveState(); 
			return
		case cmd := <-m.inbox:
			m.handleCommand(cmd)
		}
		m.saveState(); 
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

// handleMenderEvent handles events emitted by the mender manager. 
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
	var err error
	var deployed bool

	if entityType == entityTypeCoreOs && entityName != coreOSEntity {
		reply <- Result[struct{}]{Err: NewCodedError(
			ErrCodeInvalidCoreOSEntityName,
			fmt.Sprintf("invalid entity name for CoreOS: %s", entityName),
		)}
		return
	}

	e, ok := m.state.Entities[entityName]
	if !ok {
		m.state.Entities[entityName] = newEntity(entityName, entityType)
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

	deployingNV, err := e.getVersionFromArtifact(menderArtifact)
	if err != nil {
		m.logger.Errorf("Failed to get version from artifact for entity %s: %v", entityName, err)
		reply <- Result[struct{}]{Err: NewCodedError(
			ErrCodeArtifactInvalid,
			fmt.Sprintf("invalid artifact for entity %s: %v", entityName, err),
		)}
		return
	}

	if deployed, err = e.isDeployed(deployingNV); err != nil {
		m.logger.Errorf("Failed to check if artifact is already deployed for entity %s: %v. Continue", entityName, err)
		deployed = false
	}
	if deployed {
		m.logger.Infof("Received update command for entity %s, but the same version is already deployed", entityName)
		reply <- Result[struct{}]{Err: NewCodedError(
			ErrCodeAlreadyDeployed,
			fmt.Sprintf("the same version is already deployed for entity %s", entityName),
		)}
		return
	}

	e.MenderArtifact = menderArtifact
	e.DeployingNV = deployingNV

	if !m.mender.IsIdle() {
		m.logger.Infof("Received update command for entity %s, but mender is currently busy with another update", entityName)
		e.DeployStatus.Code = DeployStatusCodeWaiting
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
