/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ci4rail/moducop-core-api-server/mocks/mockmender"
)

func TestMenderCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "mender-update")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	stateDir := t.TempDir()
	t.Setenv("MOCK_MENDER_STATE_DIR", stateDir)
	t.Setenv("MOCK_MENDER_FS_ROOT", filepath.Join(stateDir, "fs"))
	t.Setenv("MOCK_MENDER_KILL_PARENT", "no")
	run := func(code int, stdoutContains, stderrContains string, args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		got := 0
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				got = e.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if got != code || !strings.Contains(stdout.String(), stdoutContains) || !strings.Contains(stderr.String(), stderrContains) {
			t.Fatalf("%v: exit %d, stdout %q, stderr %q; want exit %d containing %q / %q", args, got, stdout.String(), stderr.String(), code, stdoutContains, stderrContains)
		}
		if strings.Contains(stdout.String(), "Could not fulfill request:") {
			t.Fatal("request error was written to stdout")
		}
	}
	versionText := "5.1.0"
	idleCode, idleText := 2, "No update in progress."
	missingText, missingStderr := "Streaming failed.\nSystem not modified.\n", "Could not fulfill request:"
	rollbackText := "Committing failed.\nRolled back.\n"
	run(0, versionText, "", "--version")
	run(0, "", "mender-update resume", "--help")
	run(idleCode, idleText, "", "commit")
	run(2, "No update in progress.", "", "resume")
	run(2, "No update in progress.", "", "rollback")
	run(1, missingText, missingStderr, "install", filepath.Join(stateDir, "missing.mender"))
	st, err := mockmender.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	_, installed, trial := mockmender.Stage()
	st.Stage, st.PendingUpdateType = installed, string(mockmender.UpdateTypeRootfs)
	if err := mockmender.SaveState(st); err != nil {
		t.Fatal(err)
	}
	artifact, err := filepath.Abs("../../tests/assets/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender")
	if err != nil {
		t.Fatal(err)
	}
	run(0, "", "", "resume", "--stop-before", "ArtifactCommit_Enter")
	after, err := mockmender.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if after.Stage != installed {
		t.Fatal("stop-before committed rootfs")
	}
	run(1, "", "Update already in progress.", "install", artifact)
	run(1, rollbackText, "", "commit")
	st, err = mockmender.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	idle, _, _ := mockmender.Stage()
	if st.Stage != idle {
		t.Fatalf("rollback left stage %q", st.Stage)
	}
	st.Stage, st.PendingUpdateType, st.NewRootfs = trial, string(mockmender.UpdateTypeRootfs), "new-rootfs"
	if err := mockmender.SaveState(st); err != nil {
		t.Fatal(err)
	}
	run(0, "Committed.", "", "commit")
	st, err = mockmender.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Stage != idle || st.CommittedRootfs != "new-rootfs" {
		t.Fatalf("commit state: %+v", st)
	}
	mockmender.SetInstalledApp(&st, "interrupted.mender", "demo", "", nil)
	if err := mockmender.SaveState(st); err != nil {
		t.Fatal(err)
	}
	run(1, "", "Cannot commit from this state.", "commit")
	before, err := os.ReadFile(mockmender.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	run(1, "", "Cannot commit from this state.", "commit")
	afterBytes, err := os.ReadFile(mockmender.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterBytes) {
		t.Fatal("rejected commit modified state")
	}
	run(0, "Rolled back.", "", "rollback")
	appArtifact, err := filepath.Abs("../../tests/assets/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender")
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range []string{mockmender.ErrInjectAfterStopOldContainers, mockmender.ErrInjectAfterRenameOldAppDir, mockmender.ErrInjectAfterExtractBeforeStart} {
		if err := os.MkdirAll(mockmender.AppPath("nginx-demo"), 0755); err != nil {
			t.Fatal(err)
		}
		run(0, "Error injection set", "", "err-inject", point)
		run(1, "", "", "install", appArtifact)
		st, err := mockmender.LoadState()
		if err != nil {
			t.Fatal(err)
		}
		if st.InstallPhase == "" || st.ResumeArtifact != appArtifact {
			t.Fatalf("missing recovery checkpoint: %+v", st)
		}
		run(1, "", "Cannot commit from this state.", "commit")
		run(1, "", "Update already in progress.", "install", appArtifact)
		run(0, "Error injection cleared", "", "err-inject", "")
		run(0, "Installed, but not committed.", "", "resume", "--stop-before", "ArtifactCommit_Enter")
		pending, err := mockmender.LoadState()
		if err != nil || pending.InstallPhase != mockmender.PhaseAwaitingCommit {
			t.Fatalf("resume stop-before: %+v %v", pending, err)
		}
		run(0, "Committed.", "", "resume")
		st, err = mockmender.LoadState()
		if err != nil {
			t.Fatal(err)
		}
		if st.Stage != idle || st.InstallPhase != "" || st.ResumeArtifact != "" || len(st.RunningContainers) == 0 {
			t.Fatalf("resume did not finish rollout: %+v", st)
		}
		run(2, "No update in progress.", "", "resume")
	}
	st, err = mockmender.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	mockmender.SetInstalledApp(&st, "ready.mender", "demo", "", nil)
	st.InstallPhase = mockmender.PhaseAwaitingCommit
	if err := mockmender.SaveState(st); err != nil {
		t.Fatal(err)
	}
	run(0, "Committed.", "", "resume")
	mockmender.SetInstalledApp(&st, "committing.mender", "demo", "", nil)
	st.InstallPhase = mockmender.PhaseCommitting
	if err := mockmender.SaveState(st); err != nil {
		t.Fatal(err)
	}
	run(0, "Committed.", "", "commit")
	mockmender.SetInstalledApp(&st, "unknown.mender", "demo", "", nil)
	st.InstallPhase = "unexpected"
	if err := mockmender.SaveState(st); err != nil {
		t.Fatal(err)
	}
	run(1, "", "Cannot commit from this state.", "commit")
	run(1, "", "cannot resume unknown installation phase", "resume")
	run(0, "Rolled back.", "", "rollback")

	for _, point := range []string{mockmender.ErrInjectPostCommitFailed, mockmender.ErrInjectCleanupFailed} {
		st, err := mockmender.LoadState()
		if err != nil {
			t.Fatal(err)
		}
		st.Stage, st.PendingUpdateType, st.NewRootfs, st.ErrorInjectPoint = trial, string(mockmender.UpdateTypeRootfs), "durable-rootfs", point
		st.InstallPhase = mockmender.PhaseAwaitingCommit
		if err := mockmender.SaveState(st); err != nil {
			t.Fatal(err)
		}
		run(1, "Committed.", "", "commit")
		st, err = mockmender.LoadState()
		if err != nil {
			t.Fatal(err)
		}
		if st.Stage != idle || st.CommittedRootfs != "durable-rootfs" {
			t.Fatalf("post-commit failure undid update: %+v", st)
		}
	}

}
