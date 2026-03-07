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

type CpuManager struct {
	logger *loglite.Logger
	inbox  chan command
	quit   chan struct{}
	store  *diskstate.Store[persistenState]
	state  persistenState
	mender *menderManager
}

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

const (
	coreOSEntity = "coreos"
)

func New(persistentPath string, logLevel loglite.Level) (*CpuManager, error) {
	m := &CpuManager{
		logger: loglite.New("cpumanager", os.Stdout, logLevel),
		inbox:  make(chan command, 10),
		quit:   make(chan struct{}),
		store:  diskstate.New[persistenState](persistentPath),
	}

	// load persistent state
	if err := m.store.Load(context.Background(), &m.state); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to load persistent state: %w", err)
		}
		m.logger.Infof("Persistent state file does not exist, initializing new state")
		// initialize empty state if file does not exist
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
		m.saveState()
	}

	m.mender = newMenderManager(m.logger, &m.state.MenderState, m.emitMenderEvent)
	go m.loop()
	return m, nil
}

func (m *CpuManager) saveState() error {
	err := m.store.Save(context.Background(), m.state)
	if err != nil {
		m.logger.Errorf("Failed to save state: %v", err)
	}
	return err
}

func (m *CpuManager) loop() {
	for {
		select {
		case <-m.quit:
			return
		case cmd := <-m.inbox:
			m.handleCommand(cmd)
		}
		m.saveState()
	}
}

func (m *CpuManager) handleCommand(cmd command) {
	switch c := cmd.(type) {
	// API commands
	case StartCoreOsUpdate:
		m.handleEntityUpdate(coreOSEntity, entityTypeCoreOs, c.PathToMenderFile, c.Reply)
	case GetCoreOsState:
		//m.handleGetCoreOsState(c)
	case StartApplicationUpdate:
		m.handleEntityUpdate(c.AppName, entityTypeApplication, c.PathToMenderFile, c.Reply)
	case GetApplicationState:
		//m.handleGetApplicationState(c)
	case Reboot:
		//m.handleReboot(c)
	case MenderEvent:
		m.handleMenderEvent(c)
	default:
		m.logger.Warnf("Received unknown command: %T", cmd)
	}
}

func (m *CpuManager) handleMenderEvent(cmd MenderEvent) {
	m.logger.Infof("Handling mender event: %d", cmd.event.Code)

	switch cmd.event.Code {
	case menderEventJobFinished:
		m.logger.Infof("Mender job finished with success=%v, message=%s", cmd.event.Success, cmd.event.Message)
	default:
		m.mender.HandleEvent(cmd.event)
	}
}

func (m *CpuManager) emitMenderEvent(event menderEvent) {
	m.inbox <- MenderEvent{event: event}
}

func (m *CpuManager) handleEntityUpdate(
	entityName string,
	entityType entityType,
	menderFile string,
	reply chan Result[struct{}]) {

	if entityType == entityTypeCoreOs && entityName != coreOSEntity {
		reply <- Result[struct{}]{Err: fmt.Errorf("invalid entity name for CoreOS: %s", entityName)}
	}

	e, ok := m.state.Entities[entityName]
	if !ok {
		// create new entity
		m.state.Entities[entityName] = &entity{
			Name:         entityName,
			EntityType:   entityType,
			DeployStatus: DeployStatus{Code: DeployStatusCodeNeverDeployed, Message: "Entity has never been deployed"},
			MenderFile:   "",
		}
		e = m.state.Entities[entityName]
	}
	e.MenderFile = menderFile

	if !m.mender.IsIdle() {
		m.logger.Infof("Received update command for entity %s, but mender is currently busy with another update", entityName)
		e.DeployStatus.Code = DeployStatusCodeWaiting
		reply <- Result[struct{}]{}
		return
	}

	err := m.mender.StartUpdateJob(entityType, menderFile, 10*time.Minute)
	if err != nil {
		reply <- Result[struct{}]{Err: fmt.Errorf("failed to start mender update job for entity %s: %w", entityName, err)}
		return
	}

	reply <- Result[struct{}]{}
}
