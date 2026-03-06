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
	name         string
	entityType   entityType
	deployStatus DeployStatus
	menderFile   string // "" if no update is in progress
}

type persistenState struct {
	menderState *menderPersistentState
	entities    map[string]*entity // key: entity name, value: entity
}

const (
	coreOSEntity = "coreos"
)

func New(persistentPath string, logLevel loglite.Level) (*CpuManager, error) {
	m := &CpuManager{
		logger: loglite.New("cpumanager", os.Stdout, logLevel),
		inbox:  make(chan command),
		quit:   make(chan struct{}),
		store:  diskstate.New[persistenState](persistentPath),
	}

	// load persistent state
	if err := m.store.Load(context.Background(), &m.state); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to load persistent state: %w", err)
		}
		// initialize empty state if file does not exist
		m.state = persistenState{
			entities: make(map[string]*entity),
		}
		m.state.entities[coreOSEntity] = &entity{
			name:         coreOSEntity,
			entityType:   entityTypeCoreOs,
			deployStatus: DeployStatus{Code: DeployStatusCodeNeverDeployed, Message: "CoreOS has never been deployed"},
		}
		m.saveState()
	}
	if m.state.menderState == nil {
		m.state.menderState = &menderPersistentState{state: menderStateIdle}
	}

	m.mender = newMenderManager(m.logger, *m.state.menderState, m.emitMenderEvent)
	m.state.menderState = m.mender.persistentState()
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
		//m.handleMenderEvent(c)
	default:
		m.logger.Warnf("Received unknown command: %T", cmd)
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

	e, ok := m.state.entities[entityName]
	if !ok {
		// create new entity
		m.state.entities[entityName] = &entity{
			name:         entityName,
			entityType:   entityType,
			deployStatus: DeployStatus{Code: DeployStatusCodeNeverDeployed, Message: "Entity has never been deployed"},
			menderFile:   "",
		}
		e = m.state.entities[entityName]
	}
	e.menderFile = menderFile

	if !m.mender.IsIdle() {
		m.logger.Infof("Received update command for entity %s, but mender is currently busy with another update", entityName)
		e.deployStatus.Code = DeployStatusCodeWaiting
		reply <- Result[struct{}]{}
		return
	}

	err := m.mender.StartUpdateJob(menderFile, 10*time.Minute)
	if err != nil {
		reply <- Result[struct{}]{Err: fmt.Errorf("failed to start mender update job for entity %s: %w", entityName, err)}
		return
	}

	reply <- Result[struct{}]{}
}
