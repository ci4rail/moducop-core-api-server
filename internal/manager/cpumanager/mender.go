package cpumanager

import (
	"errors"
	"fmt"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/execcli"
	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
)

type menderState int

const (
	menderStateIdle menderState = iota
	menderStateInstalling
	menderStateRebooting
	menderStateCommitting
)

type menderEventCode int

const (
	menderEventNone menderEventCode = iota
	// internal events
	menderEventInstallFinished
	menderEventRebootFinished
	menderEventCommitFinished
	// external events
	menderEventJobFinished
)

type menderEvent struct {
	Code    menderEventCode
	Success bool
	Message string
}

type menderPersistentState struct {
	state       menderState
	currentFile string // "" if no update is in progress
}

type menderManager struct {
	state     menderPersistentState
	emitEvent func(menderEvent)
}

var (
	ErrMenderBusy = errors.New("mender is busy with another update")
)

func newMenderManager(logger *loglite.Logger, state menderPersistentState, emitEvent func(menderEvent)) *menderManager {
	return &menderManager{
		state:     state,
		emitEvent: emitEvent,
	}
}

func (m *menderManager) persistentState() *menderPersistentState {
	return &m.state
}

func (m *menderManager) StartUpdateJob(entityType entityType, file string, timeout time.Duration) error {
	if m.state.state != menderStateIdle {
		return ErrMenderBusy
	}
	m.state.state = menderStateInstalling
	m.state.currentFile = file

	go func() {
		stdout, stderr, exitCode, err := execcli.RunCommand("mender-update", timeout, "install", file)
		me := menderEvent{
			Code:    menderEventInstallFinished,
			Success: err == nil && exitCode == 0,
			Message: fmt.Sprintf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err),
		}
		m.emitEvent(me)
	}()

	return nil
}

func (m *menderManager) IsIdle() bool {
	return m.state.state == menderStateIdle
}

func (m *menderManager) HandleEvent(event menderEvent) {
	switch event.Code {
	case menderEventInstallFinished:
	}