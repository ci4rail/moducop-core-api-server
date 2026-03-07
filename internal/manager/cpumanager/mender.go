package cpumanager

import (
	"errors"
	"fmt"
	"os"
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
	UpdateResult menderUpdateResult
	Message      string
}

type menderPersistentState struct {
	State             menderState
	CurrentFile       string     // "" if no update is in progress
	CurrentEntityType entityType // valid if CurrentFile != ""
}

type menderManager struct {
	logger    *loglite.Logger
	state     *menderPersistentState
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
		menderUpdateResultInstalledAndCommited:                      "Installed and committed.",
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

func newMenderManager(logger *loglite.Logger, state *menderPersistentState, emitEvent func(menderEvent)) *menderManager {
	return &menderManager{
		logger:    logger,
		state:     state,
		emitEvent: emitEvent,
	}
}

func (m *menderManager) StartUpdateJob(entityType entityType, file string, timeout time.Duration) error {
	if m.state.State != menderStateIdle {
		return ErrMenderBusy
	}
	m.state.State = menderStateInstalling
	m.state.CurrentFile = file
	m.state.CurrentEntityType = entityType
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
			UpdateResult: result,
			Message:      fmt.Sprintf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err),
		}
		m.logger.Infof("Mender install finished: %v", me)
		m.emitEvent(me)
	}()
}

func (m *menderManager) runMenderCommitInBackGround(timeout time.Duration) {
	go func() {
		stdout, stderr, _, err := execcli.RunCommand("mender-update", timeout, "commit")
		result := m.menderUpdateResultFromInstallOutput(stdout, err)

		me := menderEvent{
			Code:         menderEventCommitFinished,
			Success:      menderUpdateResultIsSuccess(result),
			UpdateResult: result,
			Message:      fmt.Sprintf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err),
		}
		m.logger.Infof("Mender commit finished: %v", me)
		m.emitEvent(me)
	}()
}

func (m *menderManager) runRebootInBackGround(timeout time.Duration) {
	go func() {
		stdout, stderr, _, err := execcli.RunCommand("reboot", timeout)

		me := menderEvent{
			Code:    menderEventRebootFinished,
			Success: err == nil,
			Message: fmt.Sprintf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err),
		}
		m.logger.Infof("Reboot finished: %v", me)
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
	return m.state.State == menderStateIdle
}

func (m *menderManager) HandleEvent(event menderEvent) {
	m.logger.Debugf("Handling mender event: %d", event.Code)

	switch m.state.State {
	case menderStateInstalling:
		m.handleInstallingEvent(event)
	case menderStateRebooting:
		m.handleRebootingEvent(event)
	case menderStateCommitting:
		m.handleCommittingEvent(event)
	default:
		m.logger.Warnf("Received mender event in unexpected state %d: %v", m.state.State, event)
	}

}

func (m *menderManager) handleInstallingEvent(event menderEvent) {
	switch event.Code {
	case menderEventInstallFinished:
		if event.Success {
			if m.state.CurrentEntityType == entityTypeCoreOs {
				m.state.State = menderStateRebooting
				m.runRebootInBackGround(30 * time.Second)
			} else {
				if event.UpdateResult == menderUpdateResultInstalledAndCommited {
					m.setIdle()
					m.emitJobFinished(true, "")
				} else if event.UpdateResult == menderUpdateResultInstalledButNotCommited {
					m.state.State = menderStateCommitting
				} else {
					m.logger.Warnf("Received unexpected mender update result for successful install: %v", event.UpdateResult)
					m.setIdle()
					m.emitJobFinished(false, fmt.Sprintf("Unexpected mender update result: %v", event.UpdateResult))
				}
			}
		} else {
			m.setIdle()
			m.emitJobFinished(false, fmt.Sprintf("Mender update failed: %s", event.Message))
		}

	default:
		m.logger.Warnf("Received unexpected mender event code %d in installing state: %v", event.Code, event)
	}
}

func (m *menderManager) handleRebootingEvent(event menderEvent) {
	switch event.Code {
	case menderEventRebootFinished:
		if event.Success {
			m.state.State = menderStateCommitting
			m.runMenderCommitInBackGround(30 * time.Second)
		} else {
			m.setIdle()
			m.emitJobFinished(false, fmt.Sprintf("Reboot failed: %s", event.Message))
		}
	}
}

func (m *menderManager) handleCommittingEvent(event menderEvent) {
	switch event.Code {
	case menderEventCommitFinished:
		if event.Success {
			m.setIdle()
			m.emitJobFinished(true, "")
		} else {
			m.setIdle()
			m.emitJobFinished(false, fmt.Sprintf("Mender commit failed: %s", event.Message))
		}
	default:
		m.logger.Warnf("Received unexpected mender event code %d in committing state: %v", event.Code, event)
	}
}

func (m *menderManager) setIdle() {
	m.logger.Debugf("Setting mender state to idle. Current state: %+v", m.state)
	// remove current file from disk, if it exists
	if m.state.CurrentFile != "" {
		err := os.Remove(m.state.CurrentFile)
		if err != nil && !os.IsNotExist(err) {
			m.logger.Warnf("Failed to remove mender update file %s: %v", m.state.CurrentFile, err)
		}
	}
	m.state.CurrentFile = ""
	m.state.State = menderStateIdle
}

func (m *menderManager) emitJobFinished(success bool, message string) {
	m.logger.Debugf("Emitting mender job finished event. Success: %v, message: %s", success, message)
	m.emitEvent(menderEvent{
		Code:    menderEventJobFinished,
		Success: success,
		Message: message,
	})
}
