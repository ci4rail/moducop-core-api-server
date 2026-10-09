// SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
// SPDX-License-Identifier: Apache-2.0

package mockmender

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppReadiness(t *testing.T) {
	t.Setenv("MOCK_MENDER_FS_ROOT", t.TempDir())
	dir := AppManifestPath("demo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  web:\n    image: example\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, status, health        string
		paused, restarting, oneshot bool
		exit                        int
		pass                        bool
	}{
		{name: "running", status: "running", pass: true},
		{name: "healthy", status: "running", health: "healthy", pass: true},
		{name: "unhealthy", status: "running", health: "unhealthy"},
		{name: "starting", status: "running", health: "starting"},
		{name: "paused", status: "running", paused: true},
		{name: "restarting", status: "running", restarting: true},
		{name: "exited", status: "exited"},
		{name: "oneshot success", status: "exited", oneshot: true, pass: true},
		{name: "oneshot failure", status: "exited", oneshot: true, exit: 1},
		{name: "oneshot running", status: "running", oneshot: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := ContainerState{Name: "demo-web-1", Labels: "com.docker.compose.project=demo", Status: tc.status, Health: tc.health, Paused: tc.paused, Restarting: tc.restarting, ExitCode: tc.exit}
			if tc.oneshot {
				c.Labels += ",io.ci4rail.mender.oneshot=true"
			}
			s := State{PendingAppProject: "demo", AppHealth: AppHealthSettings{true, 60, 2}, RunningContainers: []ContainerState{c}}
			if err := CheckAppHealth(s); (err == nil) != tc.pass {
				t.Fatalf("health result: %v", err)
			}
		})
	}
	s := State{PendingAppProject: "demo", AppHealth: AppHealthSettings{true, 60, 2}}
	if CheckAppHealth(s) == nil {
		t.Fatal("missing container accepted")
	}
}

func TestAppHealthConfiguration(t *testing.T) {
	t.Setenv("MOCK_MENDER_FS_ROOT", t.TempDir())
	conf := MirrorPathFromAbsolute("/etc/mender/mender-app.conf")
	if err := os.MkdirAll(filepath.Dir(conf), 0755); err != nil {
		t.Fatal(err)
	}
	defaults, err := ReadAppHealthSettings()
	if err != nil || defaults != (AppHealthSettings{true, 60, 2}) {
		t.Fatalf("defaults: %+v %v", defaults, err)
	}
	for _, input := range []string{"APP_HEALTHCHECK_TIMEOUT=0", "APP_HEALTHCHECK_INTERVAL=-1", "APP_HEALTHCHECK_INTERVAL=abc", "APP_HEALTHCHECK_ENABLED=maybe"} {
		if err := os.WriteFile(conf, []byte(input), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadAppHealthSettings(); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	if err := os.WriteFile(conf, []byte("APP_HEALTHCHECK_ENABLED=no\nAPP_HEALTHCHECK_TIMEOUT=120\nAPP_HEALTHCHECK_INTERVAL=5\n"), 0644); err != nil {
		t.Fatal(err)
	}
	settings, err := ReadAppHealthSettings()
	if err != nil || settings != (AppHealthSettings{false, 120, 5}) {
		t.Fatalf("settings: %+v %v", settings, err)
	}
}

func TestRollbackRetainsMissingSnapshotTransaction(t *testing.T) {
	t.Setenv("MOCK_MENDER_FS_ROOT", t.TempDir())
	dir := AppManifestPath("demo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	s := State{PendingAppProject: "demo", PreviousAppProject: ".transactions/demo/previous", Stage: stageInstalled}
	if err := RollbackApp(&s); err == nil {
		t.Fatal("missing snapshot accepted")
	}
	if s.Stage != stageInstalled || s.PreviousAppProject == "" {
		t.Fatal("failed rollback cleared transaction")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("failed rollback removed candidate", err)
	}
}
