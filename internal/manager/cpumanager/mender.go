package cpumanager

import (
	"errors"
	"fmt"
	"strings"
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
	Code         menderEventCode
	Success      bool
	updateResult menderUpdateResult
	Message      string
}

type menderPersistentState struct {
	state       menderState
	currentFile string // "" if no update is in progress
}

type menderManager struct {
	logger    *loglite.Logger
	state     menderPersistentState
	emitEvent func(menderEvent)
}

type menderUpdateResult int

const (
	menderUpdateResultInstalledButNotCommited menderUpdateResult = iota
	menderUpdateResultInstalledAndCommited
	menderUpdateResultCommited
	menderUpdateResultInstallationFailedSystemNotModified
	menderUpdateResultInstallationFailedRolledBack
	menderUpdateResultInstallationFailedUpdateAlreadyInProgress
	menderUpdateResultInstallationFailedSystemInconsistent
	menderUpdateResultInstallationFailedGeneric
)

var (
	menderUpdateResultText = map[menderUpdateResult]string{
		menderUpdateResultInstalledButNotCommited:                   "Installed, but not committed.",
		menderUpdateResultInstalledAndCommited:                      "Installed and commited.",
		menderUpdateResultCommited:                                  "Commited.",
		menderUpdateResultInstallationFailedSystemNotModified:       "Installation failed. System not modified.",
		menderUpdateResultInstallationFailedRolledBack:              "Installation failed. Rolled back modifications.",
		menderUpdateResultInstallationFailedUpdateAlreadyInProgress: "Update already in progress.",
		menderUpdateResultInstallationFailedSystemInconsistent:      "System may be in an inconsistent state.",
		menderUpdateResultInstallationFailedGeneric:                 "",
	}
)

var (
	ErrMenderBusy = errors.New("mender is busy with another update")
)

func newMenderManager(logger *loglite.Logger, state menderPersistentState, emitEvent func(menderEvent)) *menderManager {
	return &menderManager{
		logger:    logger,
		state:     state,
		emitEvent: emitEvent,
	}
}

func (m *menderManager) persistentState() *menderPersistentState {
	return &m.state
}

func (m *menderManager) StartUpdateJob(file string, timeout time.Duration) error {
	if m.state.state != menderStateIdle {
		return ErrMenderBusy
	}
	m.state.state = menderStateInstalling
	m.state.currentFile = file
	m.runMenderInstallInBackGround(file, timeout)
	return nil
}

func (m *menderManager) runMenderInstallInBackGround(file string, timeout time.Duration) {
	go func() {
		stdout, stderr, _, err := execcli.RunCommand("mender-update", timeout, "install", file)
		result := m.menderUpdateResultFromInstallOutput(stdout, err)

		me := menderEvent{
			Code:         menderEventInstallFinished,
			Success:      menderUpdateResultIsSuccess(result),
			updateResult: result,
			Message:      fmt.Sprintf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err),
		}
		m.logger.Infof("Mender update job finished: %s", me.Message)
		m.emitEvent(me)
	}()
}

func menderUpdateResultIsSuccess(result menderUpdateResult) bool {
	return result == menderUpdateResultInstalledButNotCommited ||
		result == menderUpdateResultInstalledAndCommited ||
		result == menderUpdateResultCommited
}

func (m *menderManager) menderUpdateResultFromInstallOutput(stdout string, err error) menderUpdateResult {
	// check if stdout contains one of the texts defined in menderUpdateResultText and return the corresponding result
	for result, text := range menderUpdateResultText {
		if text != "" && strings.Contains(stdout, text) {
			successFromText := menderUpdateResultIsSuccess(result)

			if successFromText && err != nil {
				m.logger.Warnf("Mender install output indicates success, but command returned error: %v. Output: %s", err, stdout)
				return menderUpdateResultInstallationFailedGeneric
			}
			if !successFromText && err == nil {
				m.logger.Warnf("Mender install output indicates failure, but command returned success. Output: %s", stdout)
				return menderUpdateResultInstallationFailedGeneric
			}
			return result
		}
	}
	m.logger.Warnf("Mender install output did not match any known result. Output: %s, error: %v", stdout, err)
	return menderUpdateResultInstallationFailedGeneric
}

func (m *menderManager) IsIdle() bool {
	return m.state.state == menderStateIdle
}

func (m *menderManager) HandleEvent(event menderEvent) {
	switch event.Code {
	case menderEventInstallFinished:
	}
}
