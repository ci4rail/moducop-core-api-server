// SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ci4rail/moducop-core-api-server/mocks/mockmender"
)

func TestApplicationTransactions(t *testing.T) {
	t.Setenv("MOCK_MENDER_STATE_DIR", t.TempDir())
	t.Setenv("MOCK_MENDER_FS_ROOT", t.TempDir())
	t.Setenv("MOCK_MENDER_KILL_PARENT", "no")
	old := "../../tests/assets/app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9.mender"
	next := "../../tests/assets/app-nginx-demo-moducop-cpu01-linux_arm64-a895c3c.mender"
	load := func() mockmender.State {
		t.Helper()
		s, e := mockmender.LoadState()
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	check := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	install := func(path string) { t.Helper(); check(runInstall(context.Background(), path)) }
	install(old)
	s := load()
	if s.InstallPhase != mockmender.PhaseAwaitingCommit {
		t.Fatal("install must await commit")
	}
	if runInstall(context.Background(), next) == nil {
		t.Fatal("pending update accepted another install")
	}
	check(runRollback())
	if _, e := os.Stat(mockmender.AppPath("nginx-demo")); !os.IsNotExist(e) {
		t.Fatal("first-install rollback retained files")
	}
	install(old)
	check(runCommit())
	oldManifest, err := os.ReadFile(filepath.Join(mockmender.AppManifestPath("nginx-demo"), "docker-compose.yaml"))
	check(err)
	s = load()
	unrelated := mockmender.ContainerState{Name: "other-1", Labels: "com.docker.compose.project=other"}
	s.RunningContainers = append(s.RunningContainers, unrelated)
	check(mockmender.SaveState(s))
	install(next)
	check(runErrInject("app-health-unhealthy"))
	if runCommit() == nil {
		t.Fatal("unhealthy commit succeeded")
	}
	s = load()
	labels := ""
	for _, c := range s.RunningContainers {
		labels += c.Labels
	}
	if !strings.Contains(labels, "8f249b9") || !strings.Contains(labels, "project=other") || strings.Contains(labels, "a895c3c") {
		t.Fatalf("rollback containers: %+v", s.RunningContainers)
	}
	restored, err := os.ReadFile(filepath.Join(mockmender.AppManifestPath("nginx-demo"), "docker-compose.yaml"))
	check(err)
	if string(restored) != string(oldManifest) {
		t.Fatal("rollback did not restore manifests")
	}
	if s.Stage != "idle" {
		t.Fatal("rollback left transaction pending")
	}
	// Installation failures automatically roll back, allowing another attempt.
	if runInstall(context.Background(), next) == nil {
		t.Fatal("unhealthy installation succeeded")
	}
	check(runErrInject("docker-compose-up-failed"))
	if runInstall(context.Background(), next) == nil {
		t.Fatal("compose failure succeeded")
	}
	check(runErrInject(""))
	install(next)
	check(runRollback())
	for _, point := range []string{mockmender.ErrInjectAfterStopOldContainers, mockmender.ErrInjectAfterRenameOldAppDir, mockmender.ErrInjectAfterExtractBeforeStart} {
		check(runErrInject(point))
		if runInstall(context.Background(), next) == nil {
			t.Fatal("interruption ignored")
		}
		check(runRollback())
		s = load()
		if len(s.RunningContainers) != 3 {
			t.Fatalf("interrupted rollback: %+v", s)
		}
	}
	check(runErrInject(""))
	// Settings are captured at install; changing configuration cannot disable commit checks.
	install(next)
	conf := mockmender.MirrorPathFromAbsolute("/etc/mender/mender-app.conf")
	check(os.MkdirAll(filepath.Dir(conf), 0755))
	check(os.WriteFile(conf, []byte("APP_HEALTHCHECK_ENABLED=no\n"), 0644))
	check(runErrInject("app-health-missing"))
	if runCommit() == nil {
		t.Fatal("commit did not retain enabled checks")
	}
	install(next)
	check(runCommit()) // explicit no bypasses injected health failure
	if entries, e := os.ReadDir(filepath.Join(mockmender.MenderAppBasePath(), ".transactions")); e != nil || len(entries) != 0 {
		t.Fatalf("snapshot cleanup: %v %v", entries, e)
	}
}
