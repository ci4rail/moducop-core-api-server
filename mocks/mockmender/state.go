package mockmender

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultStateDir  = "/tmp/mock-mender"
	stateFileName    = "state.json"
	defaultActiveA   = "rootfs-A"
	stageIdle        = "idle"
	stageInstalled   = "installed"
	stageBootedTrial = "booted-trial"
)

const defaultIssueContent = "TDX Wayland with XWayland 7.1.0-devel-20260210154127+build.0 (scarthgap) \\n \\l\nModucop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208\n"

type State struct {
	ActiveRootfs       string           `json:"active_rootfs"`
	OldRootfs          string           `json:"old_rootfs"`
	NewRootfs          string           `json:"new_rootfs"`
	ActiveIssuePath    string           `json:"active_issue_path"`
	OldIssuePath       string           `json:"old_issue_path"`
	PendingImage       string           `json:"pending_image"`
	PendingExt4Image   string           `json:"pending_ext4_image"`
	PendingIssuePath   string           `json:"pending_issue_path"`
	PendingUpdateType  string           `json:"pending_update_type"`
	PendingAppProject  string           `json:"pending_app_project"`
	PreviousAppProject string           `json:"previous_app_project"`
	Stage              string           `json:"stage"`
	CommittedRootfs    string           `json:"committed_rootfs"`
	CommittedIssuePath string           `json:"committed_issue_path"`
	RunningContainers  []ContainerState `json:"running_containers"`
	ErrorInjectPoint   string           `json:"error_inject_point"`
	InconsistentApp    string           `json:"inconsistent_app"`
}

type ContainerState struct {
	Name   string `json:"name"`
	Labels string `json:"labels"`
}

type UpdateType string

const (
	UpdateTypeNone   UpdateType = ""
	UpdateTypeRootfs UpdateType = "rootfs-image"
	UpdateTypeApp    UpdateType = "app"
)

const (
	ErrInjectNone                    = ""
	ErrInjectAfterStopOldContainers  = "after-stop-old-containers"
	ErrInjectAfterRenameOldAppDir    = "after-renaming-old-application-directory"
	ErrInjectAfterExtractBeforeStart = "after-extracting-new-application-before-starting-new-containers"
)

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
	if err := EnsureMockFilesystem(); err != nil {
		return State{}, err
	}

	p := StatePath()
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s := State{
				ActiveRootfs:       defaultActiveA,
				CommittedRootfs:    defaultActiveA,
				CommittedIssuePath: IssueMirrorPath(),
				ActiveIssuePath:    IssueMirrorPath(),
				Stage:              stageIdle,
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
	if s.CommittedIssuePath == "" {
		s.CommittedIssuePath = IssueMirrorPath()
	}
	if s.ActiveIssuePath == "" {
		s.ActiveIssuePath = s.CommittedIssuePath
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

func SetInstalledRootfs(s *State, imageName, extractedRootfs, pendingExt4Image, pendingIssuePath string) {
	s.OldRootfs = s.ActiveRootfs
	s.OldIssuePath = s.ActiveIssuePath
	s.NewRootfs = extractedRootfs
	s.PendingImage = imageName
	s.PendingExt4Image = pendingExt4Image
	s.PendingIssuePath = pendingIssuePath
	s.PendingUpdateType = string(UpdateTypeRootfs)
	s.Stage = stageInstalled
}

func SetInstalledApp(s *State, imageName, pendingProject, previousProject string, running []ContainerState) {
	s.PendingImage = imageName
	s.PendingUpdateType = string(UpdateTypeApp)
	s.PendingAppProject = pendingProject
	s.PreviousAppProject = previousProject
	s.RunningContainers = running
	s.Stage = stageInstalled
}

func TrialBoot(s *State) {
	// First reboot after install: boot into new rootfs, but still uncommitted.
	s.ActiveRootfs = s.NewRootfs
	s.Stage = stageBootedTrial
	if s.PendingIssuePath != "" {
		s.ActiveIssuePath = s.PendingIssuePath
		if err := UpdateIssueMirror(s.PendingIssuePath); err != nil {
			// keep state transition even if mirror write fails
		}
	}
}

func RollbackAfterFailedTrial(s *State) {
	switch UpdateType(s.PendingUpdateType) {
	case UpdateTypeRootfs:
		s.ActiveRootfs = s.OldRootfs
		s.ActiveIssuePath = s.OldIssuePath
		if s.ActiveIssuePath == "" {
			s.ActiveIssuePath = s.CommittedIssuePath
		}
		if s.ActiveIssuePath != "" {
			_ = UpdateIssueMirror(s.ActiveIssuePath)
		}
	case UpdateTypeApp:
		RestorePreviousApp(s)
	}
	clearPendingUpdate(s)
}

func RollbackImmediate(s *State) {
	switch UpdateType(s.PendingUpdateType) {
	case UpdateTypeRootfs:
		if s.OldIssuePath != "" {
			s.ActiveIssuePath = s.OldIssuePath
			_ = UpdateIssueMirror(s.OldIssuePath)
		}
	case UpdateTypeApp:
		RestorePreviousApp(s)
	}
	clearPendingUpdate(s)
}

func CommitTrial(s *State) {
	s.ActiveRootfs = s.NewRootfs
	s.CommittedRootfs = s.NewRootfs
	s.CommittedIssuePath = s.PendingIssuePath
	s.ActiveIssuePath = s.PendingIssuePath
	if s.PendingIssuePath != "" {
		_ = UpdateIssueMirror(s.PendingIssuePath)
	}
	clearPendingUpdate(s)
}

func CommitApp(s *State) {
	if s.PreviousAppProject != "" {
		_ = os.RemoveAll(AppPath(s.PreviousAppProject))
	}
	clearPendingUpdate(s)
}

func clearPendingUpdate(s *State) {
	s.NewRootfs = ""
	s.OldRootfs = ""
	s.OldIssuePath = ""
	s.PendingImage = ""
	s.PendingExt4Image = ""
	s.PendingIssuePath = ""
	s.PendingUpdateType = string(UpdateTypeNone)
	s.PendingAppProject = ""
	s.PreviousAppProject = ""
	s.Stage = stageIdle
}

func MockFilesystemRoot() string {
	if v := os.Getenv("MOCK_MENDER_FS_ROOT"); v != "" {
		return v
	}
	return filepath.Join(StateDir(), "fs")
}

func MirrorPathFromAbsolute(absPath string) string {
	clean := filepath.Clean(absPath)
	clean = strings.TrimPrefix(clean, string(os.PathSeparator))
	return filepath.Join(MockFilesystemRoot(), clean)
}

func IssueMirrorPath() string {
	return MirrorPathFromAbsolute("/etc/issue")
}

func BootIDPath() string {
	return MirrorPathFromAbsolute("/proc/sys/kernel/random/boot_id")
}

func MenderAppBasePath() string {
	return MirrorPathFromAbsolute("/data/mender-app")
}

func AppPath(project string) string {
	return filepath.Join(MenderAppBasePath(), project)
}

func AppManifestPath(project string) string {
	return filepath.Join(AppPath(project), "manifest")
}

func EnsureMockFilesystem() error {
	issuePath := IssueMirrorPath()
	if err := os.MkdirAll(filepath.Dir(issuePath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(issuePath); errors.Is(err, os.ErrNotExist) {
		if writeErr := os.WriteFile(issuePath, []byte(defaultIssueContent), 0o644); writeErr != nil {
			return writeErr
		}
	} else if err != nil {
		return err
	}

	bootIDPath := BootIDPath()
	if err := os.MkdirAll(filepath.Dir(bootIDPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(bootIDPath); errors.Is(err, os.ErrNotExist) {
		if err := RotateBootID(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return os.MkdirAll(MenderAppBasePath(), 0o755)
}

func PrepareFilesystem() error {
	if err := EnsureMockFilesystem(); err != nil {
		return err
	}
	if err := os.WriteFile(IssueMirrorPath(), []byte(defaultIssueContent), 0o644); err != nil {
		return err
	}
	return RotateBootID()
}

func UpdateIssueMirror(fromPath string) error {
	if fromPath == "" {
		return nil
	}
	b, err := os.ReadFile(fromPath)
	if err != nil {
		return err
	}
	return os.WriteFile(IssueMirrorPath(), b, 0o644)
}

func RotateBootID() error {
	id, err := randomUUID()
	if err != nil {
		return err
	}
	bootIDPath := BootIDPath()
	if err := os.MkdirAll(filepath.Dir(bootIDPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(bootIDPath, []byte(id+"\n"), 0o644)
}

func randomUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func RestorePreviousApp(s *State) {
	if s.PendingAppProject == "" {
		s.RunningContainers = nil
		return
	}
	curPath := AppPath(s.PendingAppProject)
	prevPath := AppPath(fmt.Sprintf("%s-previous", s.PendingAppProject))
	if s.PreviousAppProject != "" {
		prevPath = AppPath(s.PreviousAppProject)
	}
	if _, err := os.Stat(prevPath); err == nil {
		_ = os.RemoveAll(curPath)
		_ = os.Rename(prevPath, curPath)
		running, runErr := ComposeContainersFromManifest(curPath, s.PendingAppProject)
		if runErr == nil {
			s.RunningContainers = running
			return
		}
	}
	s.RunningContainers = nil
}

func Stage() (idle, installed, trial string) {
	return stageIdle, stageInstalled, stageBootedTrial
}

func IsValidErrInjectPoint(v string) bool {
	switch v {
	case ErrInjectNone, ErrInjectAfterStopOldContainers, ErrInjectAfterRenameOldAppDir, ErrInjectAfterExtractBeforeStart:
		return true
	default:
		return false
	}
}

func RemoveRunningContainersForProject(containers []ContainerState, project string) []ContainerState {
	filtered := make([]ContainerState, 0, len(containers))
	needle := "com.docker.compose.project=" + project
	for _, c := range containers {
		if c.Labels == needle {
			continue
		}
		if strings.HasPrefix(c.Labels, needle+",") {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}
