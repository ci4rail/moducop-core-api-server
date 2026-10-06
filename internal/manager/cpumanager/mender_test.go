/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cpumanager

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
)

var errTestCommandFailed = errors.New("command failed")

func TestMenderUpdateResultFromInstallOutput(t *testing.T) {
	var logBuf bytes.Buffer
	manager := &menderManager{
		logger: loglite.New("cpumanager-test", &logBuf, loglite.Debug),
	}

	testCases := []struct {
		name   string
		stdout string
		err    error
		want   menderUpdateResult
	}{
		{
			name:   "installed but not committed",
			stdout: "Progress: 100%\nInstalled, but not committed.\nUse 'commit' to update, or 'rollback' to roll back the update.\n",
			want:   menderUpdateResultInstalledButNotCommited,
		},
		{
			name:   "installed and committed",
			stdout: "Update Module doesn't support rollback. Committing immediately.\nInstalled and committed.\n",
			want:   menderUpdateResultInstalledAndCommited,
		},
		{
			name:   "committed",
			stdout: "Committed.\n",
			want:   menderUpdateResultCommited,
		},
		{
			name:   "system not modified",
			stdout: "Installation failed. System not modified.\nCould not fulfill request: some error\n",
			want:   menderUpdateResultInstallationFailedSystemNotModified,
		},
		{
			name:   "rolled back",
			stdout: "Installation failed. Rolled back modifications.\n",
			want:   menderUpdateResultInstallationFailedRolledBack,
		},
		{
			name:   "update already in progress",
			stdout: "Could not fulfill request: Operation now in progress: Update already in progress. Please commit or roll back first\n",
			want:   menderUpdateResultInstallationFailedUpdateAlreadyInProgress,
		},
		{
			name:   "please commit or rollback",
			stdout: "Could not fulfill request: Please commit or roll back first\n",
			want:   menderUpdateResultInstallationFailedPleaseCommitOrRollback,
		},
		{
			name:   "system inconsistent",
			stdout: "Installation failed, and Update Module does not support rollback. System may be in an inconsistent state.\n",
			want:   menderUpdateResultInstallationFailedSystemInconsistent,
		},
		{
			name:   "generic fallback on unknown output",
			stdout: "some unexpected output\n",
			err:    errTestCommandFailed,
			want:   menderUpdateResultInstallationFailedGeneric,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			command, code := menderCommandInstall, 0
			if tc.name == "committed" {
				command = menderCommandCommit
			}
			if tc.err != nil {
				code = 1
			}
			got := manager.menderUpdateResultFromOutput(command, tc.stdout, "", code, tc.err)
			if got != tc.want {
				t.Fatalf("menderUpdateResultFromInstallOutput() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReserveRecoveryAttempt(t *testing.T) {
	state := &menderPersistentState{CurrentEntityType: entityTypeApplication}
	manager := &menderManager{state: state}

	for attempt := uint(1); attempt <= maxApplicationRecoveryAttempts; attempt++ {
		if !manager.reserveRecoveryAttempt() {
			t.Fatalf("reserveRecoveryAttempt() rejected attempt %d", attempt)
		}
		if state.RecoveryAttempts != attempt {
			t.Fatalf("RecoveryAttempts = %d, want %d", state.RecoveryAttempts, attempt)
		}
	}
	if manager.reserveRecoveryAttempt() {
		t.Fatal("reserveRecoveryAttempt() accepted an attempt beyond the application retry limit")
	}
}

func TestRecoveryRetryLimitFinishesJob(t *testing.T) {
	for _, eventCode := range []menderEventCode{menderEventRecoverFinished} {
		t.Run(eventCode.String(), func(t *testing.T) {
			var events []menderEvent
			state := &menderPersistentState{
				State:             menderStateInstalling,
				CurrentArtifact:   "/tmp/update.mender",
				CurrentEntityType: entityTypeApplication,
				CurrentEntityName: "demo",
				RecoveryAttempts:  maxApplicationRecoveryAttempts,
			}
			manager := &menderManager{
				logger: loglite.New("cpumanager-test", &bytes.Buffer{}, loglite.Debug),
				state:  state,
				emitEvent: func(event menderEvent) {
					events = append(events, event)
				},
				saveState: func() {},
			}

			manager.handleInstallingEvent(menderEvent{Code: eventCode, Success: true})

			if len(events) != 1 {
				t.Fatalf("received %d events, want 1", len(events))
			}
			if events[0].Code != menderEventJobFinished || events[0].Success {
				t.Fatalf("event = %+v, want failed job finished event", events[0])
			}
			if !strings.Contains(events[0].Message, "retry limit reached") {
				t.Fatalf("failure message = %q, want retry limit diagnostic", events[0].Message)
			}
			if state.State != menderStateIdle || state.RecoveryAttempts != 0 {
				t.Fatalf("state after exhaustion = %+v, want idle state with reset attempts", state)
			}
		})
	}
}

func TestMenderCommandResults(t *testing.T) {
	manager := &menderManager{logger: loglite.New("test", &bytes.Buffer{}, loglite.Debug)}
	cases := []struct {
		name, command, stdout, stderr string
		code                          int
		want                          menderUpdateResult
	}{
		{"v5 streaming failure", menderCommandInstall, "Streaming failed.\nSystem not modified.\n", "", 1, menderUpdateResultInstallationFailedSystemNotModified},
		{"v5 installation rollback", menderCommandInstall, "Installation failed.\nRolled back.\n", "", 1, menderUpdateResultInstallationFailedRolledBack},
		{"rollback succeeded", menderCommandRollback, "Rolled back.\n", "", 0, menderUpdateResultRolledBack},
		{"rollback failed", menderCommandRollback, "Rolled back.\n", "", 1, menderUpdateResultInstallationFailedRolledBack},
		{"v5 commit rollback", menderCommandCommit, "Committing failed.\nRolled back.\n", "", 1, menderUpdateResultInstallationFailedRolledBack},
		{"v5 pending", menderCommandInstall, "", "Update already in progress. Please commit or roll back first", 1, menderUpdateResultInstallationFailedUpdateAlreadyInProgress},
		{"v5 wrong state", menderCommandCommit, "", "Cannot commit from this state.", 1, menderUpdateResultWrongState},
		{"commit idle", menderCommandCommit, "No update in progress.\n", "", 2, menderUpdateResultNoUpdateInProgress},
		{"resume idle", menderCommandResume, "No update in progress.\n", "", 2, menderUpdateResultNoUpdateInProgress},
		{"install idle is unexpected", menderCommandInstall, "No update in progress.\n", "", 2, menderUpdateResultInstallationFailedGeneric},
		{"v5 resume awaiting", menderCommandResume, "Installed, but not committed.\n", "", 0, menderUpdateResultInstalledButNotCommited},
		{"resume cleanup finished", menderCommandResume, "Cleaned up.\n", "", 0, menderUpdateResultNoUpdateInProgress},
		{"v5 resume committed", menderCommandResume, "Committed.\n", "", 0, menderUpdateResultCommited},
		{"wrong command success", menderCommandCommit, "Installed, but not committed.\n", "", 0, menderUpdateResultInstallationFailedGeneric},
		{"failure overrides success", menderCommandInstall, "Installed, but not committed.\n", "", 1, menderUpdateResultInstallationFailedGeneric},
		{"stderr is not success", menderCommandCommit, "", "module log: Committed.", 0, menderUpdateResultInstallationFailedGeneric},
		{"v4 post commit", menderCommandInstall, "Installed, but one or more post-commit steps failed.\n", "", 1, menderUpdateResultCommittedWithErrors},
		{"v5 post commit", menderCommandInstall, "Installed and committed.\nOne or more post-commit steps failed.\n", "", 1, menderUpdateResultCommittedWithErrors},
		{"v5 cleanup", menderCommandCommit, "Committed.\nCleanup failed.\n", "", 1, menderUpdateResultCommittedWithErrors},
		{"committed unknown failure", menderCommandCommit, "Committed.\n", "unexpected failure", 1, menderUpdateResultCommittedWithErrors},
		{"cleanup despite zero exit", menderCommandCommit, "Committed.\nCleanup failed.\n", "", 0, menderUpdateResultCommittedWithErrors},
		{"optional reboot exit", menderCommandInstall, "Installed, but not committed.\nAt least one payload requested a reboot of the device it updated.\n", "", 4, menderUpdateResultInstalledButNotCommited},
		{"committed inconsistent failure stays committed", menderCommandCommit, "Committed.\nSystem may be in an inconsistent state.\n", "", 1, menderUpdateResultCommittedWithErrors},
		{"legacy interrupted commit", menderCommandCommit, "Committed.\nInstallation failed, and Update Module does not support rollback. System may be in an inconsistent state.\n", "", 0, menderUpdateResultInstallationFailedSystemInconsistent},
		{"auto commit reboot exit", menderCommandInstall, "Installed and committed.\nAt least one payload requested a reboot of the device it updated.\n", "", 4, menderUpdateResultInstalledAndCommited},
		{"signal death", menderCommandInstall, "Installed, but not committed.\n", "", -1, menderUpdateResultInstallationFailedGeneric},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.code != 0 {
				err = errTestCommandFailed
			}
			got := manager.menderUpdateResultFromOutput(tc.command, tc.stdout, tc.stderr, tc.code, err)
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCommittedErrorsFinishWithoutRecovery(t *testing.T) {
	for _, stateCode := range []menderState{menderStateInstalling, menderStateCommitting, menderStateRecoverInstallCommitting} {
		t.Run(stateCode.String(), func(t *testing.T) {
			var events []menderEvent
			state := &menderPersistentState{State: stateCode, CurrentEntityType: entityTypeApplication, CurrentEntityName: "demo"}
			manager := &menderManager{logger: loglite.New("test", &bytes.Buffer{}, loglite.Debug), state: state,
				saveState: func() {}, emitEvent: func(e menderEvent) { events = append(events, e) }}
			code := menderEventCommitFinished
			if stateCode == menderStateInstalling {
				code = menderEventInstallFinished
			}
			manager.HandleEvent(menderEvent{Code: code, UpdateResult: menderUpdateResultCommittedWithErrors, Message: "Cleanup failed."})
			if len(events) != 1 || events[0].Code != menderEventJobFinished || events[0].Success || !strings.Contains(events[0].Message, "Cleanup failed.") || state.State != menderStateIdle {
				t.Fatalf("events %+v, state %+v", events, state)
			}
		})
	}
}

func TestRecoveryOperationsAreBounded(t *testing.T) {
	var events []menderEvent
	state := &menderPersistentState{State: menderStateRecoverInstallCommitting, RecoveryOperations: maxRecoveryOperations}
	manager := &menderManager{logger: loglite.New("test", &bytes.Buffer{}, loglite.Debug), state: state,
		saveState: func() {}, emitEvent: func(e menderEvent) { events = append(events, e) }}
	manager.HandleEvent(menderEvent{Code: menderEventCommitFinished, UpdateResult: menderUpdateResultWrongState})
	if len(events) != 1 || events[0].Success || state.State != menderStateIdle {
		t.Fatalf("events %+v, state %+v", events, state)
	}
}

func TestResumeCapabilityDetection(t *testing.T) {
	for _, tc := range []struct {
		name, help string
		supported  bool
	}{
		{"mender4", "install commit rollback", false},
		{"mender5", "install resume commit rollback", true},
		{"substring is insufficient", "cannot-resume", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			script := "#!/bin/sh\n[ \"$1\" = --help ] || exit 1\nprintf '%s\\n' '" + tc.help + "' >&2\n"
			if err := os.WriteFile(filepath.Join(dir, "mender-update"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			manager := &menderManager{logger: loglite.New("test", &bytes.Buffer{}, loglite.Debug)}
			if got := manager.supportsResume(); got != tc.supported {
				t.Fatalf("got %v, want %v", got, tc.supported)
			}
		})
	}
}

func TestFailedRecoveryFinishesJob(t *testing.T) {
	var events []menderEvent
	state := &menderPersistentState{State: menderStateInstalling, CurrentEntityType: entityTypeCoreOs}
	manager := &menderManager{logger: loglite.New("test", &bytes.Buffer{}, loglite.Debug), state: state,
		saveState: func() {}, emitEvent: func(e menderEvent) { events = append(events, e) }}
	manager.HandleEvent(menderEvent{Code: menderEventRecoverFinished, Message: "system in inconsistent state"})
	if len(events) != 1 || events[0].Success || events[0].Message != "system in inconsistent state" || state.State != menderStateIdle {
		t.Fatalf("events %+v, state %+v", events, state)
	}
}

func TestResumeKeepsRebootAndHealthChecksBeforeCommit(t *testing.T) {
	for _, typ := range []entityType{entityTypeCoreOs, entityTypeCoreOSCustomization, entityTypeApplication} {
		manager := &menderManager{state: &menderPersistentState{CurrentEntityType: typ}}
		args := manager.resumeInstallArgs()
		if typ == entityTypeApplication {
			if len(args) != 0 {
				t.Fatal(args)
			}
			continue
		}
		if strings.Join(args, " ") != "--stop-before ArtifactCommit_Enter" {
			t.Fatal(args)
		}
	}
}

func TestNoPendingCommitRequiresRequestedVersion(t *testing.T) {
	artifact, err := os.ReadFile("../../../tests/assets/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"8f249b9", "different-version"} {
		t.Run(version, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("MOCK_MENDER_STATE_DIR", root)
			manifestDir := filepath.Join(root, "fs/data/mender-app/nginx-demo/manifests")
			if err := os.MkdirAll(manifestDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(manifestDir, ".env"), []byte("SOFTWARE_VERSION="+version+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			upload := filepath.Join(root, "update.mender")
			if err := os.WriteFile(upload, artifact, 0600); err != nil {
				t.Fatal(err)
			}
			var events []menderEvent
			state := &menderPersistentState{State: menderStateCommitting, CurrentEntityType: entityTypeApplication, CurrentEntityName: "nginx-demo", CurrentArtifact: upload}
			manager := &menderManager{logger: loglite.New("test", &bytes.Buffer{}, loglite.Debug), state: state,
				saveState: func() {}, emitEvent: func(e menderEvent) { events = append(events, e) }}
			manager.HandleEvent(menderEvent{Code: menderEventCommitFinished, Command: menderCommandCommit, UpdateResult: menderUpdateResultNoUpdateInProgress})
			if len(events) != 1 || events[0].Success != (version == "8f249b9") || state.State != menderStateIdle {
				t.Fatalf("events %+v, state %+v", events, state)
			}
			if _, err := os.Stat(filepath.Join(manifestDir, ".env")); err != nil {
				t.Fatal("completion removed deployed files", err)
			}
		})
	}
}

func TestSilentResumeRequiresExplicitCommitStop(t *testing.T) {
	manager := &menderManager{logger: loglite.New("test", &bytes.Buffer{}, loglite.Debug)}
	got := manager.menderUpdateResultFromOutput(menderCommandResume, "", "", 0, nil, "--stop-before", "ArtifactCommit_Enter")
	if got != menderUpdateResultInstalledButNotCommited {
		t.Fatalf("got %s", got)
	}
	got = manager.menderUpdateResultFromOutput(menderCommandResume, "", "", 0, nil)
	if got != menderUpdateResultInstallationFailedGeneric {
		t.Fatalf("silent unqualified resume: %s", got)
	}
}
