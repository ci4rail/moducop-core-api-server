package cpumanager

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/execcli"
	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
	"github.com/ci4rail/moducop-core-api-server/internal/prefixfs"
)

type menderState int

const (
	menderStateIdle menderState = iota
	menderStateInstalling
	menderStateRebooting
	menderStateCommitting
	menderStateRecoverInstallCommitting
	menderStateRecoverInstallClearApp
)

type menderEventCode int

const (
	menderEventNone menderEventCode = iota
	// internal events
	menderEventInstallFinished
	menderEventRebootFinished
	menderEventCommitFinished
	menderEventRestarted
	menderEventRecoverFinished
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
	CurrentArtifact   string     // "" if no update is in progress
	CurrentEntityType entityType // valid if CurrentArtifact != ""
	CurrentEntityName string     // valid if CurrentArtifact != ""
}

type menderManager struct {
	logger    *loglite.Logger
	state     *menderPersistentState
	emitEvent func(menderEvent)
	saveState func()
}

type menderUpdateResult int

const (
	menderUpdateResultInstalledButNotCommited menderUpdateResult = iota
	menderUpdateResultInstallationFailedPleaseCommitOrRollback
	menderUpdateResultInstallationFailedSystemInconsistent
	menderUpdateResultInstallationFailedSystemNotModified
	menderUpdateResultInstallationFailedRolledBack
	menderUpdateResultInstallationFailedUpdateAlreadyInProgress
	menderUpdateResultInstalledAndCommited
	menderUpdateResultCommited
	// must be last!
	menderUpdateResultInstallationFailedGeneric
)

const (
	rebootTimeout = 30 * time.Second
	commitTimeout = 30 * time.Second
	rebootDelay   = 3 * time.Second
)

var (
	ErrMenderBusy = errors.New("mender is busy with another update")
)

func newMenderManager(logger *loglite.Logger, state *menderPersistentState, emitEvent func(menderEvent), hasRebooted bool, saveState func()) *menderManager {
	m := &menderManager{
		logger:    logger,
		state:     state,
		emitEvent: emitEvent,
		saveState: saveState,
	}
	if hasRebooted && m.state.State == menderStateRebooting {
		m.emitRebootFinished("System reboot detected", nil)
	} else {
		m.emitEvent(menderEvent{
			Code: menderEventRestarted,
		})
	}
	return m
}

func (m *menderManager) StartUpdateJob(entityType entityType, artifact string, entityName string, timeout time.Duration) error {
	if m.state.State != menderStateIdle {
		return ErrMenderBusy
	}
	m.state.State = menderStateInstalling
	m.state.CurrentArtifact = artifact
	m.state.CurrentEntityType = entityType
	m.state.CurrentEntityName = entityName
	m.runMenderInstallInBackGround(artifact, timeout)
	return nil
}

func (m *menderManager) runMenderInstallInBackGround(artifact string, timeout time.Duration) {
	m.saveState()
	go func() {
		stdout, stderr, _, err := execcli.RunCommandWithLogger("mender-update", timeout, m.logger, "install", artifact)
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

// nolint: unparam
func (m *menderManager) runMenderCommitInBackGround(timeout time.Duration) {
	m.saveState()
	go func() {
		stdout, stderr, _, err := execcli.RunCommandWithLogger("mender-update", timeout, m.logger, "commit")
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
	m.saveState()
	go func() {
		time.Sleep(rebootDelay)
		stdout, stderr, _, err := execcli.RunCommandWithLogger("reboot", timeout, m.logger)
		if err != nil {
			message := fmt.Sprintf("Reboot command failed: stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			m.emitRebootFinished(message, err)
		}
	}()
}

func menderUpdateResultIsSuccess(result menderUpdateResult) bool {
	return result == menderUpdateResultInstalledButNotCommited ||
		result == menderUpdateResultInstalledAndCommited ||
		result == menderUpdateResultCommited
}

func (m *menderManager) menderUpdateResultFromInstallOutput(stdout string, err error) menderUpdateResult {
	for result := menderUpdateResultInstalledButNotCommited; result < menderUpdateResultInstallationFailedGeneric; result++ {
		text := menderUpdateResultText(result)
		if text != "" && strings.Contains(stdout, text) {
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
	m.logger.Debugf("Handling mender event: %s", event.Code)

	switch m.state.State {
	case menderStateIdle:
		m.logger.Warnf("Received mender event while idle: %v", event)
	case menderStateInstalling:
		m.handleInstallingEvent(event)
	case menderStateRebooting:
		m.handleRebootingEvent(event)
	case menderStateCommitting:
		m.handleCommittingEvent(event)
	case menderStateRecoverInstallCommitting:
		m.handleRecoverInstallCommittingEvent(event)
	case menderStateRecoverInstallClearApp:
		m.handleRecoverInstallClearAppEvent(event)
	default:
		m.logger.Warnf("Received mender event in unexpected state %d: %v", m.state.State, event)
	}
}

func (m *menderManager) handleInstallingEvent(event menderEvent) {
	switch event.Code {
	case menderEventNone, menderEventRebootFinished, menderEventCommitFinished, menderEventJobFinished:
		m.logger.Warnf("Received unexpected mender event code %s in installing state: %v", event.Code, event)
	case menderEventRestarted:
		m.logger.Infof("Mender manager restarted while installing. Restarting install")
		m.runMenderInstallInBackGround(m.state.CurrentArtifact, updateTimeout)
	case menderEventRecoverFinished:
		m.logger.Infof("Recovery install finished while installing. Restarting install")
		m.runMenderInstallInBackGround(m.state.CurrentArtifact, updateTimeout)
	case menderEventInstallFinished:
		switch event.UpdateResult {
		case menderUpdateResultInstalledButNotCommited:
			if m.state.CurrentEntityType == entityTypeCoreOs {
				m.state.State = menderStateRebooting
				m.runRebootInBackGround(rebootTimeout)
			} else {
				m.state.State = menderStateCommitting
				m.runMenderCommitInBackGround(commitTimeout)
			}
		case menderUpdateResultInstalledAndCommited, menderUpdateResultCommited:
			m.emitJobFinished(true, "")

		case menderUpdateResultInstallationFailedSystemInconsistent, menderUpdateResultInstallationFailedPleaseCommitOrRollback:
			m.logger.Warnf("Mender reported inconsistent system or pending commit/rollback after installation. Starting recovery install.")
			m.startRecoverInstall()
		case menderUpdateResultInstallationFailedSystemNotModified,
			menderUpdateResultInstallationFailedRolledBack,
			menderUpdateResultInstallationFailedUpdateAlreadyInProgress,
			menderUpdateResultInstallationFailedGeneric:
			m.logger.Warnf("Received unexpected mender update result for successful install: %v", event.UpdateResult)
			m.emitJobFinished(false, fmt.Sprintf("Unexpected mender update result: %s", event.UpdateResult))
		}
	}
}

func (m *menderManager) handleRebootingEvent(event menderEvent) {
	switch event.Code {
	case menderEventRestarted:
		m.logger.Infof("System reboot detected. Retrying reboot to complete installation.")
		m.runRebootInBackGround(rebootTimeout)
	case menderEventRebootFinished:
		if event.Success {
			m.state.State = menderStateCommitting
			m.runMenderCommitInBackGround(commitTimeout)
		} else {
			m.logger.Warnf("Reboot failed during installation. Starting recovery install.")
			m.emitJobFinished(false, "Could not reboot")
		}
	case menderEventNone, menderEventInstallFinished, menderEventCommitFinished, menderEventJobFinished, menderEventRecoverFinished:
		m.logger.Warnf("Received unexpected mender event code %s in rebooting state: %v", event.Code, event)
	}
}

func (m *menderManager) handleCommittingEvent(event menderEvent) {
	switch event.Code {
	case menderEventRestarted:
		m.logger.Infof("System reboot detected. Retrying commit to complete installation.")
		m.runMenderCommitInBackGround(commitTimeout)

	case menderEventCommitFinished:
		// TODO: recovery needed?
		if event.Success {
			m.emitJobFinished(true, "")
		} else {
			m.emitJobFinished(false, fmt.Sprintf("Mender commit failed: %s", event.UpdateResult))
		}
	case menderEventNone, menderEventInstallFinished, menderEventRebootFinished, menderEventJobFinished, menderEventRecoverFinished:
		m.logger.Warnf("Received unexpected mender event code %s in committing state: %v", event.Code, event)
	}
}

func (m *menderManager) handleRecoverInstallCommittingEvent(event menderEvent) {
	switch event.Code {
	case menderEventRestarted:
		m.logger.Infof("Mender manager restarted while in recover install. Restarting recovery install")
		m.startRecoverInstall()
	case menderEventCommitFinished:
		switch event.UpdateResult {
		case menderUpdateResultInstalledAndCommited, menderUpdateResultCommited:
			m.state.State = menderStateInstalling
			m.emitRecoverfinished(true, "")
		case menderUpdateResultInstallationFailedPleaseCommitOrRollback:
			m.logger.Warnf("Recovery commit failed with pending commit/rollback. Starting recovery install again.")
			m.startRecoverInstall()
		case menderUpdateResultInstallationFailedSystemInconsistent:
			if m.state.CurrentEntityType == entityTypeApplication {
				m.clearAppDir()
			} else {
				m.emitRecoverfinished(false, "system in inconsistent state")
			}
		case menderUpdateResultInstallationFailedSystemNotModified,
			menderUpdateResultInstallationFailedRolledBack,
			menderUpdateResultInstallationFailedUpdateAlreadyInProgress,
			menderUpdateResultInstalledButNotCommited,
			menderUpdateResultInstallationFailedGeneric:
			m.logger.Warnf("Received unexpected mender update result for failed commit in recovery install: %v", event.UpdateResult)
			m.emitRecoverfinished(false, fmt.Sprintf("Unexpected mender update result during recovery commit: %s", event.UpdateResult))
		}
	case menderEventNone, menderEventInstallFinished, menderEventRebootFinished, menderEventJobFinished, menderEventRecoverFinished:
		m.logger.Warnf("Received unexpected mender event code %s in recover install state: %v", event.Code, event)
	}
}

func (m *menderManager) handleRecoverInstallClearAppEvent(event menderEvent) {
	switch event.Code {
	case menderEventRestarted:
		m.logger.Infof("Mender manager restarted while in recover install clear app. Restarting recovery install")
		m.clearAppDir()
	case menderEventNone, menderEventRecoverFinished, menderEventCommitFinished, menderEventInstallFinished, menderEventRebootFinished, menderEventJobFinished:
		m.logger.Warnf("Received unexpected mender event code %s in recover install clear app state: %v", event.Code, event)
	}
}

func (m *menderManager) setIdle() {
	m.logger.Debugf("Setting mender state to idle. Current state: %+v", m.state)
	// remove current file from disk, if it exists
	if m.state.CurrentArtifact != "" {
		err := os.Remove(m.state.CurrentArtifact)
		if err != nil && !os.IsNotExist(err) {
			m.logger.Warnf("Failed to remove mender update file %s: %v", m.state.CurrentArtifact, err)
		}
	}
	m.state.CurrentArtifact = ""
	m.state.CurrentEntityType = entityTypeCoreOs
	m.state.CurrentEntityName = ""
	m.state.State = menderStateIdle
	m.saveState()
}

func (m *menderManager) emitJobFinished(success bool, message string) {
	m.setIdle()
	m.logger.Debugf("Emitting mender job finished event. Success: %v, message: %s", success, message)
	m.emitEvent(menderEvent{
		Code:    menderEventJobFinished,
		Success: success,
		Message: message,
	})
}

func (m *menderManager) emitRebootFinished(message string, err error) {
	me := menderEvent{
		Code:    menderEventRebootFinished,
		Success: err == nil,
		Message: message,
	}
	m.logger.Infof("Reboot finished: %v", me)
	m.emitEvent(me)
}

func (m *menderManager) emitRecoverfinished(success bool, message string) {
	me := menderEvent{
		Code:    menderEventRecoverFinished,
		Success: success,
		Message: message,
	}
	m.logger.Infof("Recovery install finished: %v", me)
	m.emitEvent(me)
}

func (m *menderManager) startRecoverInstall() {
	m.logger.Infof("Starting recovery install for entity %s of type %s", m.state.CurrentEntityName, m.state.CurrentEntityType)
	m.state.State = menderStateRecoverInstallCommitting
	m.runMenderCommitInBackGround(commitTimeout)
}

func (m *menderManager) clearAppDir() {
	m.logger.Infof("Clearing app directory for app %s as part of recovery install", m.state.CurrentEntityName)
	m.state.State = menderStateRecoverInstallClearApp
	appName := m.state.CurrentEntityName
	appDirExtensions := []string{"", "-previous", "-last"}

	for _, ext := range appDirExtensions {
		appDir := fmt.Sprintf("%s/%s%s", prefixfs.Path(menderAppRootDir), appName, ext)
		err := os.RemoveAll(appDir)
		if err != nil {
			m.logger.Warnf("Failed to clear app directory %s: %v", appDir, err)
		}
	}
	m.state.State = menderStateInstalling
	m.emitRecoverfinished(true, "")
}

func (me *menderEvent) String() string {
	return fmt.Sprintf("{Code: %d, Success: %v, UpdateResult: %s, Message: %s}", me.Code, me.Success, me.UpdateResult, me.Message)
}

// These texts are reported to called
// nolint: cyclop
func (r menderUpdateResult) String() string {
	switch r {
	case menderUpdateResultInstalledButNotCommited:
		return "Installed, but not committed."
	case menderUpdateResultInstalledAndCommited:
		return "Installed and committed."
	case menderUpdateResultCommited:
		return "Committed."
	case menderUpdateResultInstallationFailedSystemNotModified:
		return "Installation failed. System not modified."
	case menderUpdateResultInstallationFailedRolledBack:
		return "Installation failed. Rolled back modifications."
	case menderUpdateResultInstallationFailedUpdateAlreadyInProgress:
		return "Update already in progress."
	case menderUpdateResultInstallationFailedPleaseCommitOrRollback:
		return "Please commit or roll back first"
	case menderUpdateResultInstallationFailedSystemInconsistent:
		return "System may be in an inconsistent state."
	case menderUpdateResultInstallationFailedGeneric:
		return "Installation failed. Generic error."
	default:
		return fmt.Sprintf("Unknown result: %d", r)
	}
}

// These texts are checked in the mender-update outputs to map it to result codes
// nolint: cyclop
func menderUpdateResultText(result menderUpdateResult) string {
	switch result {
	case menderUpdateResultInstalledButNotCommited:
		return "Installed, but not committed."
	case menderUpdateResultInstalledAndCommited:
		return "Installed and committed."
	case menderUpdateResultCommited:
		return "Committed."
	case menderUpdateResultInstallationFailedSystemNotModified:
		return "Installation failed. System not modified."
	case menderUpdateResultInstallationFailedRolledBack:
		return "Installation failed. Rolled back modifications."
	case menderUpdateResultInstallationFailedUpdateAlreadyInProgress:
		return "Update already in progress."
	case menderUpdateResultInstallationFailedPleaseCommitOrRollback:
		return "Please commit or roll back first"
	case menderUpdateResultInstallationFailedSystemInconsistent:
		return "System may be in an inconsistent state."
	case menderUpdateResultInstallationFailedGeneric:
		return ""
	}
	return ""
}

func (c menderEventCode) String() string {
	switch c {
	case menderEventNone:
		return "None"
	case menderEventInstallFinished:
		return "InstallFinished"
	case menderEventRebootFinished:
		return "RebootFinished"
	case menderEventCommitFinished:
		return "CommitFinished"
	case menderEventRestarted:
		return "Restarted"
	case menderEventRecoverFinished:
		return "RecoverFinished"
	case menderEventJobFinished:
		return "JobFinished"
	default:
		return fmt.Sprintf("Unknown event code: %d", c)
	}
}
