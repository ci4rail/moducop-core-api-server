/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cpumanager

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		{"committed inconsistent summary", menderCommandCommit, "Committed.\nInstallation failed, and Update Module does not support rollback. System may be in an inconsistent state.\n", "", 0, menderUpdateResultCommittedWithErrors},
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

func TestApplicationCommitTimeout(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{{"", 5 * time.Minute}, {"90s", 90 * time.Second}, {"invalid", 5 * time.Minute}, {"0s", 5 * time.Minute}, {"-1s", 5 * time.Minute}} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("MENDER_APPLICATION_COMMIT_TIMEOUT", tc.value)
			m := &menderManager{logger: loglite.New("test", &bytes.Buffer{}, loglite.Debug), state: &menderPersistentState{CurrentEntityType: entityTypeApplication}}
			if got := m.commitCommandTimeout(); got != tc.want {
				t.Fatalf("timeout %s, want %s", got, tc.want)
			}
			m.state.CurrentEntityType = entityTypeCoreOs
			if got := m.commitCommandTimeout(); got != commitTimeout {
				t.Fatalf("rootfs timeout %s", got)
			}
		})
	}
}

//nolint:cyclop // Table-driven lifecycle tests verify terminal results and preservation of every recovery file.
func TestApplicationLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   menderState
		event   menderEvent
		script  string
		success bool
	}{
		{"pending install commits", menderStateInstalling, menderEvent{Code: menderEventInstallFinished, UpdateResult: menderUpdateResultInstalledButNotCommited}, "[ \"$1\" = commit ] || exit 10\nprintf 'Committed.\\n'", true},
		{"commit health check rolls back", menderStateInstalling, menderEvent{Code: menderEventInstallFinished, UpdateResult: menderUpdateResultInstalledButNotCommited}, "[ \"$1\" = commit ] || exit 10\nprintf 'Committing failed.\\nRolled back.\\n'\nexit 1", false},
		{"commit health and rollback fail", menderStateInstalling, menderEvent{Code: menderEventInstallFinished, UpdateResult: menderUpdateResultInstalledButNotCommited}, "case \"$1\" in commit|rollback) printf 'System may be in an inconsistent state.\\n'; exit 1;; *) exit 10;; esac", false},
		{"health failure rolled back", menderStateCommitting, menderEvent{Code: menderEventCommitFinished, UpdateResult: menderUpdateResultInstallationFailedRolledBack}, "exit 10", false},
		{"failed commit rolls back", menderStateCommitting, menderEvent{Code: menderEventCommitFinished, UpdateResult: menderUpdateResultInstallationFailedGeneric}, "[ \"$1\" = rollback ] || exit 10\nprintf 'Rolled back.\\n'", false},
		{"failed rollback preserves files", menderStateInstalling, menderEvent{Code: menderEventInstallFinished, UpdateResult: menderUpdateResultInstallationFailedSystemInconsistent}, "[ \"$1\" = rollback ] || exit 10\nprintf 'System may be in an inconsistent state.\\n'\nexit 1", false},
		{"interrupted install resumes", menderStateInstalling, menderEvent{Code: menderEventRestarted}, "[ \"$1\" = resume ] || exit 10\nprintf 'Installed and committed.\\n'", true},
		{"interrupted recovery resumes", menderStateRecoverInstallCommitting, menderEvent{Code: menderEventRestarted}, "[ \"$1\" = resume ] || exit 10\nprintf 'Installed and committed.\\n'", true},
		{"interrupted commit resumes", menderStateCommitting, menderEvent{Code: menderEventRestarted}, "[ \"$1\" = resume ] || exit 10\nprintf 'Committed.\\n'", true},
		{"interrupted rollback", menderStateRollingBackApplication, menderEvent{Code: menderEventRestarted}, "[ \"$1\" = rollback ] || exit 10\nprintf 'Rolled back.\\n'", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("MOCK_MENDER_STATE_DIR", root)
			t.Setenv("PATH", root)
			if err := os.WriteFile(filepath.Join(root, "mender-update"), []byte("#!/bin/sh\n"+tc.script+"\n"), 0755); err != nil {
				t.Fatal(err)
			}
			files := []string{"demo/manifests/.env", "demo-previous/snapshot", "demo-last/snapshot", ".transactions/demo/snapshot"}
			for _, file := range files {
				path := filepath.Join(root, "fs/data/mender-app", file)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			events := make(chan menderEvent, 10)
			state := &menderPersistentState{State: tc.state, CurrentEntityType: entityTypeApplication, CurrentEntityName: "demo"}
			m := &menderManager{logger: loglite.New("test", io.Discard, loglite.Debug), state: state, saveState: func() {}, emitEvent: func(e menderEvent) { events <- e }}
			m.HandleEvent(tc.event)
			deadline := time.After(3 * time.Second)
		loop:
			for {
				select {
				case e := <-events:
					if e.Code == menderEventJobFinished {
						if e.Success != tc.success {
							t.Fatalf("result %+v", e)
						}
						break loop
					}
					// A wrong command must fail the test, even when job failure is expected.
					if strings.Contains(e.Message, "exit 10,") {
						t.Fatalf("unexpected command: %+v", e)
					}
					m.HandleEvent(e)
				case <-deadline:
					t.Fatal("no terminal event")
				}
			}
			if state.State != menderStateIdle {
				t.Fatalf("state %v", state.State)
			}
			for _, file := range files {
				data, err := os.ReadFile(filepath.Join(root, "fs/data/mender-app", file))
				if err != nil || string(data) != "preserve" {
					t.Fatalf("file %s changed: %q %v", file, data, err)
				}
			}
		})
	}
}
