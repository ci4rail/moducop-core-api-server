/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cpumanager

import (
	"bytes"
	"errors"
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
			got := manager.menderUpdateResultFromInstallOutput(tc.stdout, tc.err)
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
