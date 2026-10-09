/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cpumanager

import (
	"encoding/json"
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
	menderStateRecoverInstallCommitting
	menderStateVerifyingCustomization
	menderStateRollingBackCustomization
	menderStateRollingBackApplication
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
	menderEventCustomizationVerified
	menderEventRollbackFinished
	// external events
	menderEventJobFinished
)

type menderEvent struct {
	Code         menderEventCode
	Success      bool
	UpdateResult menderUpdateResult
	Message      string
	Command      string // install, resume, commit, or rollback
}

type menderPersistentState struct {
	State              menderState
	CurrentArtifact    string     // "" if no update is in progress
	CurrentEntityType  entityType // valid if CurrentArtifact != ""
	CurrentEntityName  string     // valid if CurrentArtifact != ""
	RecoveryOperations uint       // bounded commit/resume recovery operations
	RecoveryAttempts   uint       // recovery re-installs started for the current update
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
	menderUpdateResultCommittedWithErrors
	menderUpdateResultNoUpdateInProgress
	menderUpdateResultWrongState
	menderUpdateResultRolledBack
	// must be last!
	menderUpdateResultInstallationFailedGeneric
)

const (
	menderCommandInstall            = "install"
	menderCommandCommit             = "commit"
	menderCommandResume             = "resume"
	menderCommandRollback           = "rollback"
	maxRecoveryOperations           = 2
	rebootTimeout                   = 30 * time.Second
	commitTimeout                   = 30 * time.Second
	defaultApplicationCommitTimeout = 5 * time.Minute
	customizationStatusTimeout      = 10 * time.Second
	customizationStatusPollInterval = time.Second
	customizationStatusMaxWait      = 5 * time.Minute
	rebootDelay                     = 3 * time.Second

	// An application artifact cannot change while its update is in progress. A
	// retry can recover an interrupted rollout, but retrying it indefinitely
	// cannot fix a deterministic error such as an invalid bind mount.
	maxApplicationRecoveryAttempts = 1
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

// Application health checks, rollback and cleanup share this command budget.
func (m *menderManager) commitCommandTimeout() time.Duration {
	if m.state.CurrentEntityType != entityTypeApplication {
		return commitTimeout
	}
	if value := os.Getenv("MENDER_APPLICATION_COMMIT_TIMEOUT"); value != "" {
		timeout, err := time.ParseDuration(value)
		if err == nil && timeout > 0 {
			return timeout
		}
		m.logger.Warnf("Invalid MENDER_APPLICATION_COMMIT_TIMEOUT %q; using %s", value, defaultApplicationCommitTimeout)
	}
	return defaultApplicationCommitTimeout
}

func (m *menderManager) startApplicationRollback() {
	m.state.State = menderStateRollingBackApplication
	m.runMenderRollbackInBackGround(m.commitCommandTimeout())
}

func (m *menderManager) handleRollingBackApplicationEvent(event menderEvent) {
	switch event.Code {
	case menderEventRestarted:
		m.startApplicationRollback()
	case menderEventRollbackFinished:
		if event.UpdateResult == menderUpdateResultRolledBack ||
			event.UpdateResult == menderUpdateResultInstallationFailedRolledBack {
			m.emitJobFinished(false, "Application update failed; Mender rolled back the update")
		} else {
			m.emitJobFinished(false, "Application update failed; Mender rollback could not be confirmed; application manifests and snapshots preserved: "+event.Message)
		}
	case menderEventNone, menderEventInstallFinished, menderEventRebootFinished, menderEventCommitFinished, menderEventRecoverFinished, menderEventCustomizationVerified, menderEventJobFinished:
		m.logger.Warnf("Unexpected event while rolling back application: %v", event)
	}
}

func (m *menderManager) StartUpdateJob(entityType entityType, artifact string, entityName string, timeout time.Duration) error {
	if m.state.State != menderStateIdle {
		return ErrMenderBusy
	}
	m.state.State = menderStateInstalling
	m.state.CurrentArtifact = artifact
	m.state.CurrentEntityType = entityType
	m.state.CurrentEntityName = entityName
	m.state.RecoveryAttempts = 0
	m.state.RecoveryOperations = 0
	m.runMenderInstallInBackGround(artifact, timeout)
	return nil
}

func (m *menderManager) runMenderInstallInBackGround(artifact string, timeout time.Duration) {
	m.saveState()
	go m.runMenderCommand(menderCommandInstall, timeout, menderEventInstallFinished, artifact)
}

func (m *menderManager) runMenderCommand(command string, timeout time.Duration, code menderEventCode, args ...string) {
	stdout, stderr, exitCode, err := execcli.RunCommandWithLogger("mender-update", timeout, m.logger, append([]string{command}, args...)...)
	result := m.menderUpdateResultFromOutput(command, stdout, stderr, exitCode, err, args...)
	event := menderEvent{Code: code, Command: command, Success: menderUpdateResultIsSuccess(result), UpdateResult: result,
		Message: fmt.Sprintf("mender-update %s: exit %d, error: %v; %s%s", command, exitCode, err, stdout, stderr)}
	m.logger.Infof("Mender command finished: %v", event)
	m.emitEvent(event)
}

// A resumed rootfs/customization install must still pass through our reboot
// and health-check steps before Mender commits it.
func (m *menderManager) resumeInstallArgs() []string {
	if m.state.CurrentEntityType != entityTypeApplication {
		return []string{"--stop-before", "ArtifactCommit_Enter"}
	}
	return nil
}

func (m *menderManager) runInterruptedInstallInBackground() {
	m.saveState()
	go m.runMenderCommand(menderCommandResume, updateTimeout, menderEventInstallFinished, m.resumeInstallArgs()...)
}

// reserveRecoveryAttempt records a recovery-triggered application re-install.
// The value is persistent so restarting core-api-server cannot reset the retry
// budget after application recovery has started.
func (m *menderManager) reserveRecoveryAttempt() bool {
	if m.state.CurrentEntityType == entityTypeApplication &&
		m.state.RecoveryAttempts >= maxApplicationRecoveryAttempts {
		return false
	}
	m.state.RecoveryAttempts++
	return true
}

// nolint: unparam
func (m *menderManager) runMenderCommitInBackGround(timeout time.Duration) {
	m.saveState()
	go m.runMenderCommand(menderCommandCommit, timeout, menderEventCommitFinished)
}

func (m *menderManager) runMenderRollbackInBackGround(timeout time.Duration) {
	m.saveState()
	go m.runMenderCommand(menderCommandRollback, timeout, menderEventRollbackFinished)
}

//nolint:nestif // Polling distinguishes pending, successful, and failed health-check states.
func (m *menderManager) runCustomizationStatusVerificationInBackground() {
	m.saveState()
	go func() {
		deadline := time.Now().Add(customizationStatusMaxWait)
		for {
			stdout, stderr, _, err := execcli.RunCommandWithLogger("os-customization-set", customizationStatusTimeout, m.logger, "status")
			if err == nil {
				var status struct {
					ActiveVersion  *string `json:"active_version"`
					CandidateSlot  *string `json:"candidate_slot"`
					CandidateState *string `json:"candidate_state"`
				}
				if err := json.Unmarshal([]byte(stdout), &status); err == nil {
					if status.CandidateSlot == nil || status.CandidateState == nil || *status.CandidateState != "pending" {
						if status.CandidateState == nil && status.ActiveVersion != nil {
							m.emitEvent(menderEvent{Code: menderEventCustomizationVerified, Success: true})
							return
						}
						m.emitEvent(menderEvent{Code: menderEventCustomizationVerified, Success: false, Message: "Core OS customization health checks failed"})
						return
					}
				} else {
					m.logger.Warnf("Invalid os-customization-set status output: %v", err)
				}
			} else {
				m.logger.Warnf("os-customization-set status failed: %v: %s", err, stderr)
			}
			if time.Now().After(deadline) {
				m.emitEvent(menderEvent{Code: menderEventCustomizationVerified, Success: false, Message: "Timed out waiting for Core OS customization health checks"})
				return
			}
			time.Sleep(customizationStatusPollInterval)
		}
	}()
}

func (m *menderManager) runRebootInBackGround(timeout time.Duration) {
	m.saveState()
	go func() {
		time.Sleep(rebootDelay)
		_, _, _, err := execcli.RunCommandWithLogger("reboot", timeout, m.logger)
		if err != nil {
			message := fmt.Sprintf("Reboot command failed: err: %v", err)
			m.emitRebootFinished(message, err)
		}
	}()
}

func menderUpdateResultIsSuccess(result menderUpdateResult) bool {
	return result == menderUpdateResultInstalledButNotCommited ||
		result == menderUpdateResultInstalledAndCommited ||
		result == menderUpdateResultCommited || result == menderUpdateResultRolledBack
}

// Classify terminal CLI summaries, keeping diagnostics separate from success lines.
// Mender 5 prints failure and disposition on separate lines.
//
//nolint:cyclop // Ordered command and disposition checks keep committed failures out of destructive recovery.
func (m *menderManager) menderUpdateResultFromOutput(command, stdout, stderr string, exitCode int, err error, args ...string) menderUpdateResult {
	output := stdout + "\n" + stderr
	contains := func(text string) bool { return strings.Contains(output, text) }
	line := func(text string) bool { return menderSummaryLine(stdout, text) }
	diagnostics := []struct {
		text   string
		result menderUpdateResult
	}{
		{"Update already in progress.", menderUpdateResultInstallationFailedUpdateAlreadyInProgress},
		{"Cannot commit from this state.", menderUpdateResultWrongState},
		{"Please commit or roll back first", menderUpdateResultInstallationFailedPleaseCommitOrRollback},
	}
	for _, diagnostic := range diagnostics {
		if contains(diagnostic.text) {
			return diagnostic.result
		}
	}

	if menderNoUpdate(command, stdout, exitCode) {
		return menderUpdateResultNoUpdateInProgress
	}
	rebootRequested := (command == menderCommandInstall || command == menderCommandResume) && exitCode == 4 &&
		contains("At least one payload requested a reboot of the device it updated.")
	committed := menderCommittedSummary(stdout)
	postCommitFailure := contains("post-commit steps failed.") || contains("Cleanup failed.")
	if committed && (postCommitFailure ||
		((exitCode != 0 || err != nil) && !rebootRequested) || contains("System may be in an inconsistent state.")) {
		return menderUpdateResultCommittedWithErrors
	}
	if command == menderCommandRollback && exitCode == 0 && err == nil && line("Rolled back.") {
		return menderUpdateResultRolledBack
	}
	dispositions := []struct {
		text   string
		result menderUpdateResult
	}{
		{"System may be in an inconsistent state.", menderUpdateResultInstallationFailedSystemInconsistent},
		{"System not modified.", menderUpdateResultInstallationFailedSystemNotModified},
		{"Rolled back modifications.", menderUpdateResultInstallationFailedRolledBack},
		{"Rolled back.", menderUpdateResultInstallationFailedRolledBack},
	}
	for _, disposition := range dispositions {
		if contains(disposition.text) {
			return disposition.result
		}
	}

	if (exitCode != 0 || err != nil) && !rebootRequested {
		return menderUpdateResultInstallationFailedGeneric
	}
	if result := menderSuccessSummary(command, stdout); result != menderUpdateResultInstallationFailedGeneric {
		return result
	}
	// Resuming an already installed transaction at this explicit stop point
	// can succeed silently: Mender reports only work performed in this invocation.
	if menderPausedBeforeCommit(command, stdout, exitCode, err, args) {
		return menderUpdateResultInstalledButNotCommited
	}
	m.logger.Warnf("Unrecognized Mender %s result: exit %d, error %v, output %s", command, exitCode, err, output)
	return menderUpdateResultInstallationFailedGeneric
}

func menderCommittedSummary(stdout string) bool {
	for _, summary := range []string{"Committed.", "Installed and committed.", "Installed, but one or more post-commit steps failed."} {
		if menderSummaryLine(stdout, summary) {
			return true
		}
	}
	return false
}

func menderPausedBeforeCommit(command, stdout string, exitCode int, err error, args []string) bool {
	return command == menderCommandResume && exitCode == 0 && err == nil && strings.TrimSpace(stdout) == "" &&
		len(args) == 2 && args[0] == "--stop-before" && args[1] == "ArtifactCommit_Enter"
}

func menderNoUpdate(command, stdout string, exitCode int) bool {
	return (command == menderCommandCommit || command == menderCommandResume || command == menderCommandRollback) &&
		exitCode == 2 && menderSummaryLine(stdout, "No update in progress.")
}

func menderSummaryLine(stdout, text string) bool {
	for _, value := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(value) == text {
			return true
		}
	}
	return false
}

func menderSuccessSummary(command, stdout string) menderUpdateResult {
	switch command {
	case menderCommandInstall, menderCommandResume:
		if menderSummaryLine(stdout, "Installed, but not committed.") {
			return menderUpdateResultInstalledButNotCommited
		}
		if menderSummaryLine(stdout, "Installed and committed.") {
			return menderUpdateResultInstalledAndCommited
		}
		if command == menderCommandResume {
			if menderSummaryLine(stdout, "Committed.") {
				return menderUpdateResultCommited
			}
			if menderSummaryLine(stdout, "Cleaned up.") {
				return menderUpdateResultNoUpdateInProgress
			}
		}
	case menderCommandCommit:
		if menderSummaryLine(stdout, "Committed.") {
			return menderUpdateResultCommited
		}
		if menderSummaryLine(stdout, "Installed and committed.") {
			return menderUpdateResultInstalledAndCommited
		}
	}
	return menderUpdateResultInstallationFailedGeneric
}

// A process may have completed before the server could persist its completion.
// No pending transaction alone is insufficient evidence that this job succeeded.
func (m *menderManager) currentArtifactIsDeployed() bool {
	e := &entity{Name: m.state.CurrentEntityName, EntityType: m.state.CurrentEntityType}
	expected, err := e.getVersionFromArtifact(m.state.CurrentArtifact)
	if err != nil {
		return false
	}
	deployed, err := e.isDeployed(expected)
	return err == nil && deployed
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
	case menderStateVerifyingCustomization:
		m.handleVerifyingCustomizationEvent(event)
	case menderStateRollingBackApplication:
		m.handleRollingBackApplicationEvent(event)
	case menderStateRollingBackCustomization:
		m.handleRollingBackCustomizationEvent(event)
	default:
		m.logger.Warnf("Received mender event in unexpected state %d: %v", m.state.State, event)
	}
}

func (m *menderManager) handleInstallingEvent(event menderEvent) {
	switch event.Code {
	case menderEventNone, menderEventRebootFinished, menderEventCommitFinished, menderEventJobFinished, menderEventCustomizationVerified, menderEventRollbackFinished:
		m.logger.Warnf("Received unexpected mender event code %s in installing state: %v", event.Code, event)
	case menderEventRestarted:
		m.logger.Infof("Mender manager restarted while installing. Recovering interrupted installation")
		m.runInterruptedInstallInBackground()
	case menderEventRecoverFinished:
		if !event.Success {
			m.emitJobFinished(false, event.Message)
			return
		}
		m.logger.Infof("Recovery install finished while installing. Restarting install")
		m.runMenderRecoveryInstallInBackGround()
	case menderEventInstallFinished:
		if event.UpdateResult == menderUpdateResultCommittedWithErrors {
			m.emitJobFinished(false, "Mender committed the update, but subsequent steps failed: "+event.Message)
			return
		}
		if event.UpdateResult == menderUpdateResultNoUpdateInProgress {
			if m.currentArtifactIsDeployed() {
				m.emitJobFinished(true, "")
				return
			}
			// No transaction was created before the interruption. Start the requested
			// artifact once, using the same persistent recovery budget as other retries.
			m.runMenderRecoveryInstallInBackGround()
			return
		}
		m.handleInstallFinished(event)
	}
}

func (m *menderManager) runMenderRecoveryInstallInBackGround() {
	if !m.reserveRecoveryAttempt() {
		m.emitJobFinished(false, fmt.Sprintf(
			"application installation failed after %d recovery retry; recovery retry limit reached",
			maxApplicationRecoveryAttempts,
		))
		return
	}
	m.runMenderInstallInBackGround(m.state.CurrentArtifact, updateTimeout)
}

func (m *menderManager) handleInstallFinished(event menderEvent) {
	if m.state.CurrentEntityType == entityTypeApplication && event.UpdateResult == menderUpdateResultInstallationFailedGeneric {
		m.startApplicationRollback()
		return
	}
	switch event.UpdateResult {
	case menderUpdateResultInstalledButNotCommited:
		m.startPostInstallStep()
	case menderUpdateResultInstalledAndCommited, menderUpdateResultCommited:
		m.emitJobFinished(true, "")
	case menderUpdateResultInstallationFailedSystemInconsistent:
		m.maybeRecoverApplication()
	case menderUpdateResultWrongState, menderUpdateResultInstallationFailedPleaseCommitOrRollback,
		menderUpdateResultInstallationFailedUpdateAlreadyInProgress:
		m.logger.Warnf("Mender reported inconsistent system or pending commit/rollback after installation. Starting recovery install.")
		m.startRecoverInstall()
	case menderUpdateResultRolledBack, menderUpdateResultCommittedWithErrors, menderUpdateResultNoUpdateInProgress, menderUpdateResultInstallationFailedSystemNotModified,
		menderUpdateResultInstallationFailedRolledBack,
		menderUpdateResultInstallationFailedGeneric:
		m.logger.Warnf("Mender install/resume failed: %v", event.UpdateResult)
		m.emitJobFinished(false, fmt.Sprintf("Mender %s failed: %s; %s", event.Command, event.UpdateResult, event.Message))
	}
}

func (m *menderManager) startPostInstallStep() {
	if m.state.CurrentEntityType == entityTypeCoreOs || m.state.CurrentEntityType == entityTypeCoreOSCustomization {
		m.state.State = menderStateRebooting
		m.runRebootInBackGround(rebootTimeout)
		return
	}
	m.state.State = menderStateCommitting
	m.runMenderCommitInBackGround(m.commitCommandTimeout())
}

func (m *menderManager) handleRebootingEvent(event menderEvent) {
	switch event.Code {
	case menderEventRestarted:
		m.logger.Infof("System reboot detected. Retrying reboot to complete installation.")
		m.runRebootInBackGround(rebootTimeout)
	case menderEventRebootFinished:
		if event.Success {
			if m.state.CurrentEntityType == entityTypeCoreOSCustomization {
				m.state.State = menderStateVerifyingCustomization
				m.runCustomizationStatusVerificationInBackground()
			} else {
				m.state.State = menderStateCommitting
				m.runMenderCommitInBackGround(m.commitCommandTimeout())
			}
		} else {
			m.logger.Warnf("Reboot failed during installation. Starting recovery install.")
			m.emitJobFinished(false, "Could not reboot")
		}
	case menderEventNone, menderEventInstallFinished, menderEventCommitFinished, menderEventJobFinished, menderEventRecoverFinished, menderEventCustomizationVerified, menderEventRollbackFinished:
		m.logger.Warnf("Received unexpected mender event code %s in rebooting state: %v", event.Code, event)
	}
}

func (m *menderManager) handleVerifyingCustomizationEvent(event menderEvent) {
	switch event.Code {
	case menderEventRestarted:
		m.runCustomizationStatusVerificationInBackground()
	case menderEventCustomizationVerified:
		if event.Success {
			m.state.State = menderStateCommitting
			m.runMenderCommitInBackGround(m.commitCommandTimeout())
			return
		}
		m.state.State = menderStateRollingBackCustomization
		m.runMenderRollbackInBackGround(commitTimeout)
	case menderEventNone, menderEventInstallFinished, menderEventRebootFinished, menderEventCommitFinished, menderEventJobFinished, menderEventRecoverFinished, menderEventRollbackFinished:
		m.logger.Warnf("Received unexpected mender event code %s while verifying customization: %v", event.Code, event)
	}
}

func (m *menderManager) handleRollingBackCustomizationEvent(event menderEvent) {
	switch event.Code {
	case menderEventRestarted:
		m.runMenderRollbackInBackGround(commitTimeout)
	case menderEventRollbackFinished:
		if event.Success || event.UpdateResult == menderUpdateResultNoUpdateInProgress {
			m.emitJobFinished(false, "Core OS customization health checks failed")
		} else {
			m.emitJobFinished(false, "Core OS customization health checks failed and Mender rollback failed")
		}
	case menderEventNone, menderEventInstallFinished, menderEventRebootFinished, menderEventCommitFinished, menderEventJobFinished, menderEventRecoverFinished, menderEventCustomizationVerified:
		m.logger.Warnf("Received unexpected mender event code %s while rolling back customization: %v", event.Code, event)
	}
}

//nolint:cyclop // Distinguish pending, committed, rolled back and interrupted command outcomes.
func (m *menderManager) handleCommittingEvent(event menderEvent) {
	switch event.Code {
	case menderEventRestarted:
		m.logger.Infof("Resuming interrupted commit")
		m.saveState()
		go m.runMenderCommand(menderCommandResume, m.commitCommandTimeout(), menderEventCommitFinished)

	case menderEventCommitFinished:
		if event.UpdateResult == menderUpdateResultInstalledButNotCommited {
			m.runMenderCommitInBackGround(m.commitCommandTimeout())
			return
		}
		if m.state.CurrentEntityType == entityTypeApplication &&
			(event.UpdateResult == menderUpdateResultInstallationFailedGeneric ||
				event.UpdateResult == menderUpdateResultInstallationFailedSystemInconsistent ||
				event.UpdateResult == menderUpdateResultInstallationFailedPleaseCommitOrRollback) {
			m.startApplicationRollback()
			return
		}
		if event.UpdateResult == menderUpdateResultNoUpdateInProgress && m.currentArtifactIsDeployed() {
			m.emitJobFinished(true, "")
			return
		}
		if event.UpdateResult == menderUpdateResultWrongState {
			m.startRecoverInstall()
			return
		}
		if event.UpdateResult == menderUpdateResultCommittedWithErrors {
			m.emitJobFinished(false, "Mender committed the update, but subsequent steps failed: "+event.Message)
			return
		}
		if event.UpdateResult == menderUpdateResultCommited || event.UpdateResult == menderUpdateResultInstalledAndCommited {
			m.emitJobFinished(true, "")
		} else {
			m.emitJobFinished(false, fmt.Sprintf("Mender commit failed: %s; %s", event.UpdateResult, event.Message))
		}
	case menderEventNone, menderEventInstallFinished, menderEventRebootFinished, menderEventJobFinished, menderEventRecoverFinished, menderEventCustomizationVerified, menderEventRollbackFinished:
		m.logger.Warnf("Received unexpected mender event code %s in committing state: %v", event.Code, event)
	}
}

func (m *menderManager) handleRecoverInstallCommittingEvent(event menderEvent) {
	switch event.Code {
	case menderEventRestarted:
		m.logger.Infof("Mender manager restarted while in recover install. Restarting recovery install")
		m.startRecoverInstall()
	case menderEventCommitFinished:
		m.state.State = menderStateInstalling
		m.handleInstallingEvent(menderEvent{Code: menderEventInstallFinished, Command: menderCommandResume, UpdateResult: event.UpdateResult, Success: event.Success, Message: event.Message})
	case menderEventNone, menderEventInstallFinished, menderEventRebootFinished, menderEventJobFinished, menderEventRecoverFinished, menderEventCustomizationVerified, menderEventRollbackFinished:
		m.logger.Warnf("Received unexpected mender event code %s in recover install state: %v", event.Code, event)
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
	m.state.RecoveryAttempts = 0
	m.state.RecoveryOperations = 0
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

func (m *menderManager) startRecoverInstall() {
	if m.state.RecoveryOperations >= maxRecoveryOperations {
		m.emitJobFinished(false, "Mender recovery retry limit reached")
		return
	}
	m.state.RecoveryOperations++
	m.state.State = menderStateRecoverInstallCommitting
	m.saveState()
	go m.runMenderCommand(menderCommandResume, updateTimeout, menderEventCommitFinished, m.resumeInstallArgs()...)
}

func (m *menderManager) maybeRecoverApplication() {
	if m.state.CurrentEntityType == entityTypeApplication {
		m.startApplicationRollback()
	} else {
		m.emitJobFinished(false, "system in inconsistent state")
	}
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
	case menderUpdateResultRolledBack:
		return "Rolled back."
	case menderUpdateResultCommittedWithErrors:
		return "Update committed, but post-commit or cleanup steps failed."
	case menderUpdateResultNoUpdateInProgress:
		return "No update in progress."
	case menderUpdateResultWrongState:
		return "Cannot commit from this state."
	case menderUpdateResultInstallationFailedGeneric:
		return "Installation failed. Generic error."
	default:
		return fmt.Sprintf("Unknown result: %d", r)
	}
}

//nolint:cyclop // Each event has a distinct diagnostic name.
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
	case menderEventCustomizationVerified:
		return "CustomizationVerified"
	case menderEventRollbackFinished:
		return "RollbackFinished"
	default:
		return fmt.Sprintf("Unknown event code: %d", c)
	}
}
