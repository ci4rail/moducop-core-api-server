// SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
// SPDX-License-Identifier: Apache-2.0

package mockmender

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type AppHealthSettings struct {
	Enabled  bool `json:"enabled"`
	Timeout  int  `json:"timeout"`
	Interval int  `json:"interval"`
}

func ValidAppName(name string) bool {
	return regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`).MatchString(name)
}

func ReadAppHealthSettings() (AppHealthSettings, error) {
	s := AppHealthSettings{true, 60, 2}
	b, err := os.ReadFile(MirrorPathFromAbsolute("/etc/mender/mender-app.conf"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		switch strings.TrimSpace(key) {
		case "APP_HEALTHCHECK_ENABLED":
			if value != "yes" && value != "no" {
				return s, fmt.Errorf("invalid APP_HEALTHCHECK_ENABLED")
			}
			s.Enabled = value == "yes"
		case "APP_HEALTHCHECK_TIMEOUT", "APP_HEALTHCHECK_INTERVAL":
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return s, fmt.Errorf("invalid %s", key)
			}
			if key == "APP_HEALTHCHECK_TIMEOUT" {
				s.Timeout = n
			} else {
				s.Interval = n
			}
		}
	}
	return s, nil
}

func HasProject(c ContainerState, project string) bool {
	for _, label := range strings.Split(c.Labels, ",") {
		if label == "com.docker.compose.project="+project {
			return true
		}
	}
	return false
}

// Readiness is deterministic: simulated states represent the final result of
// polling, without waiting the target's entire timeout or executing probes.
func CheckAppHealth(s State) error {
	if !s.AppHealth.Enabled {
		return nil
	}
	expected, err := ComposeContainersFromManifest(AppPath(s.PendingAppProject), s.PendingAppProject)
	if err != nil {
		return err
	}
	if len(expected) == 0 {
		return fmt.Errorf("application readiness failed: no containers")
	}
	for _, want := range expected {
		found := false
		for _, c := range s.RunningContainers {
			if c.Name != want.Name {
				continue
			}
			found = true
			switch s.ErrorInjectPoint {
			case "app-health-unhealthy":
				c.Health = "unhealthy"
			case "app-health-starting":
				c.Health = "starting"
			case "app-health-exited":
				c.Status = "exited"
				c.ExitCode = 1
			case "app-health-paused":
				c.Paused = true
			case "app-health-restarting":
				c.Restarting = true
			case "app-health-missing":
				found = false
			case "app-health-oneshot-failed":
				c.Status = "exited"
				c.ExitCode = 1
				c.Labels += ",io.ci4rail.mender.oneshot=true"
			}
			oneshot := strings.Contains(","+c.Labels+",", ",io.ci4rail.mender.oneshot=true,")
			ready := (c.Status == "" || c.Status == "running") && !c.Paused && !c.Restarting && (c.Health == "" || c.Health == "healthy")
			if oneshot {
				ready = c.Status == "exited" && c.ExitCode == 0
			}
			if !ready || !found {
				return fmt.Errorf("application readiness timed out after %ds: %s", s.AppHealth.Timeout, c.Name)
			}
		}
		if !found {
			return fmt.Errorf("application readiness failed: missing container %s", want.Name)
		}
	}
	return nil
}

func CleanupAppSnapshot(s *State) error {
	if s.PreviousAppProject == "" {
		return nil
	}
	if err := os.RemoveAll(AppPath(s.PreviousAppProject)); err != nil {
		return err
	}
	if strings.HasPrefix(s.PreviousAppProject, ".transactions/") {
		return os.RemoveAll(filepath.Dir(AppPath(s.PreviousAppProject)))
	}
	return nil
}

func RollbackApp(s *State) error {
	project := s.PendingAppProject
	if s.PreviousAppProject != "" {
		if _, err := os.Stat(AppPath(s.PreviousAppProject)); err != nil {
			return err
		}
		if err := os.RemoveAll(AppPath(project)); err != nil {
			return err
		}
		if err := os.Rename(AppPath(s.PreviousAppProject), AppPath(project)); err != nil {
			return err
		}
		if err := CleanupAppSnapshot(s); err != nil {
			return err
		}
	} else if s.InstallPhase != PhaseInstalling && s.InstallPhase != PhaseAppStopped {
		if err := os.RemoveAll(AppPath(project)); err != nil {
			return err
		}
	}
	s.RunningContainers = append(RemoveRunningContainersForProject(s.RunningContainers, project), s.PreviousContainers...)
	clearPendingUpdate(s)
	return nil
}

// Every managed service must still have a container to snapshot, even if stopped.
func CheckAppSnapshotSource(s State, project string) error {
	expected, err := ComposeContainersFromManifest(AppPath(project), project)
	if os.IsNotExist(err) {
		for _, c := range s.RunningContainers {
			if HasProject(c, project) {
				return fmt.Errorf("unmanaged Compose project already exists: %s", project)
			}
		}
		return nil
	}
	if err != nil {
		return err
	}
	for _, want := range expected {
		found := false
		for _, c := range s.RunningContainers {
			if c.Name == want.Name && HasProject(c, project) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("cannot snapshot missing container: %s", want.Name)
		}
	}
	return nil
}
