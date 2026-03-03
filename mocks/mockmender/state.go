package mockmender

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	defaultStateDir  = "/tmp/mock-mender"
	stateFileName    = "state.json"
	defaultActiveA   = "rootfs-A"
	stageIdle        = "idle"
	stageInstalled   = "installed"
	stageBootedTrial = "booted-trial"
)

type State struct {
	ActiveRootfs       string `json:"active_rootfs"`
	OldRootfs          string `json:"old_rootfs"`
	NewRootfs          string `json:"new_rootfs"`
	PendingImage       string `json:"pending_image"`
	PendingExt4Image   string `json:"pending_ext4_image"`
	PendingIssuePath   string `json:"pending_issue_path"`
	Stage              string `json:"stage"`
	CommittedRootfs    string `json:"committed_rootfs"`
	CommittedIssuePath string `json:"committed_issue_path"`
}

func StateDir() string {
	if v := os.Getenv("MOCK_MENDER_STATE_DIR"); v != "" {
		return v
	}
	return defaultStateDir
}

func StatePath() string {
	return filepath.Join(StateDir(), stateFileName)
}

func LoadState() (State, error) {
	p := StatePath()
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s := State{
				ActiveRootfs:    defaultActiveA,
				CommittedRootfs: defaultActiveA,
				Stage:           stageIdle,
			}
			if saveErr := SaveState(s); saveErr != nil {
				return State{}, saveErr
			}
			return s, nil
		}
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	if s.ActiveRootfs == "" {
		s.ActiveRootfs = defaultActiveA
	}
	if s.CommittedRootfs == "" {
		s.CommittedRootfs = s.ActiveRootfs
	}
	if s.Stage == "" {
		s.Stage = stageIdle
	}
	return s, nil
}

func SaveState(s State) error {
	if err := os.MkdirAll(StateDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StatePath(), b, 0o600)
}

func SetInstalled(s *State, imageName, extractedRootfs, pendingExt4Image, pendingIssuePath string) {
	s.OldRootfs = s.ActiveRootfs
	s.NewRootfs = extractedRootfs
	s.PendingImage = imageName
	s.PendingExt4Image = pendingExt4Image
	s.PendingIssuePath = pendingIssuePath
	s.Stage = stageInstalled
}

func TrialBoot(s *State) {
	// First reboot after install: boot into new rootfs, but still uncommitted.
	s.ActiveRootfs = s.NewRootfs
	s.Stage = stageBootedTrial
}

func RollbackAfterFailedTrial(s *State) {
	s.ActiveRootfs = s.OldRootfs
	s.NewRootfs = ""
	s.OldRootfs = ""
	s.PendingImage = ""
	s.PendingExt4Image = ""
	s.PendingIssuePath = ""
	s.Stage = stageIdle
}

func RollbackImmediate(s *State) {
	s.NewRootfs = ""
	s.OldRootfs = ""
	s.PendingImage = ""
	s.PendingExt4Image = ""
	s.PendingIssuePath = ""
	s.Stage = stageIdle
}

func CommitTrial(s *State) {
	s.ActiveRootfs = s.NewRootfs
	s.CommittedRootfs = s.NewRootfs
	s.CommittedIssuePath = s.PendingIssuePath
	s.NewRootfs = ""
	s.OldRootfs = ""
	s.PendingImage = ""
	s.PendingExt4Image = ""
	s.PendingIssuePath = ""
	s.Stage = stageIdle
}

func Stage() (idle, installed, trial string) {
	return stageIdle, stageInstalled, stageBootedTrial
}
