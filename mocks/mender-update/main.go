package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ci4rail/moducop-core-api-server/mocks/mockmender"
)

const expectedDeviceType = "moducop-cpu01"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "install":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		if err := runInstall(context.Background(), os.Args[2]); err != nil {
			os.Exit(1)
		}
	case "commit":
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		if err := runCommit(); err != nil {
			os.Exit(1)
		}
	case "rollback":
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		if err := runRollback(); err != nil {
			os.Exit(1)
		}
	case "show-issue":
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		if err := runShowIssue(); err != nil {
			os.Exit(1)
		}
	case "err-inject":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		if err := runErrInject(os.Args[2]); err != nil {
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  mender-update install <image-file>")
	fmt.Fprintln(os.Stderr, "  mender-update commit")
	fmt.Fprintln(os.Stderr, "  mender-update rollback")
	fmt.Fprintln(os.Stderr, "  mender-update show-issue")
	fmt.Fprintln(os.Stderr, "  mender-update err-inject <none|after-stop-old-containers|after-renaming-old-application-directory|after-extracting-new-application-before-starting-new-containers>")
}

func runInstall(_ context.Context, imagePath string) error {
	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}
	_, installed, trial := mockmender.Stage()
	if _, err := os.Stat(imagePath); err != nil {
		name := filepath.Base(imagePath)
		fmt.Printf("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"No such file or directory: Failed to open '%s' for reading\"\n", name)
		fmt.Println("Installation failed. System not modified.")
		fmt.Printf("Could not fulfill request: No such file or directory: Failed to open '%s' for reading\n", name)
		return err
	}

	info, metadata, err := mockmender.ParseArtifactHeader(imagePath)
	if err != nil {
		fmt.Printf("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
		fmt.Println("Installation failed. System not modified.")
		fmt.Printf("Could not fulfill request: %s\n", err.Error())
		return err
	}

	if len(info.ArtifactDepends.DeviceType) == 0 || info.ArtifactDepends.DeviceType[0] != expectedDeviceType {
		fmt.Println("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Artifact device type doesn't match\"")
		fmt.Println("Installation failed. System not modified.")
		return fmt.Errorf("device type mismatch")
	}
	if len(info.Payloads) == 0 {
		fmt.Println("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Unsupported payload type\"")
		fmt.Println("Installation failed. System not modified.")
		return fmt.Errorf("unsupported payload")
	}

	switch info.Payloads[0].Type {
	case string(mockmender.UpdateTypeRootfs):
		if st.Stage == installed || st.Stage == trial {
			msg := "Operation now in progress: Update already in progress. Please commit or roll back first"
			fmt.Printf("record_id=1 severity=error time=\"2026-Mar-03 07:09:26.999642\" name=\"Global\" msg=\"%s\"\n", msg)
			fmt.Println("Installation failed. System not modified.")
			fmt.Printf("Could not fulfill request: %s\n", msg)
			return errors.New(msg)
		}
		return installRootfs(&st, imagePath)
	case string(mockmender.UpdateTypeApp), "docker-compose":
		if err := failIfAppStateInconsistent(&st, metadata); err != nil {
			return err
		}
		if st.Stage == installed || st.Stage == trial {
			msg := "Operation now in progress: Update already in progress. Please commit or roll back first"
			fmt.Println("Installation failed. System not modified.")
			fmt.Printf("Could not fulfill request: %s\n", msg)
			return errors.New(msg)
		}
		return installApp(&st, imagePath, metadata)
	default:
		fmt.Println("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Unsupported payload type\"")
		fmt.Println("Installation failed. System not modified.")
		return fmt.Errorf("unsupported payload")
	}
}

func installRootfs(st *mockmender.State, imagePath string) error {
	info, extractedRootfs, err := mockmender.ParseAndExtractArtifact(imagePath)
	if err != nil {
		fmt.Printf("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
		fmt.Println("Installation failed. System not modified.")
		fmt.Printf("Could not fulfill request: %s\n", err.Error())
		return err
	}
	if len(info.Payloads) == 0 || info.Payloads[0].Type != string(mockmender.UpdateTypeRootfs) {
		fmt.Println("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Unsupported payload type\"")
		fmt.Println("Installation failed. System not modified.")
		return fmt.Errorf("unsupported payload")
	}

	for i := 1; i <= 10; i++ {
		fmt.Printf("Progress: %d%%\n", i*10)
		time.Sleep(1 * time.Second)
	}

	ext4ImagePath, issuePath, err := mockmender.PrepareRootfsInspection(extractedRootfs)
	if err != nil {
		fmt.Printf("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
		fmt.Println("Installation failed. System not modified.")
		fmt.Printf("Could not fulfill request: %s\n", err.Error())
		return err
	}
	mockmender.SetInstalledRootfs(st, filepath.Base(imagePath), extractedRootfs, ext4ImagePath, issuePath)
	if err := mockmender.SaveState(*st); err != nil {
		return err
	}

	fmt.Println("Installed, but not committed.")
	fmt.Println("Use 'commit' to update, or 'rollback' to roll back the update.")
	fmt.Printf("New rootfs ext4 image: %s\n", ext4ImagePath)
	if issuePath != "" {
		fmt.Printf("Extracted /etc/issue: %s\n", issuePath)
		fmt.Println("Use `mender-update show-issue` to print it.")
	} else {
		fmt.Println("Could not extract /etc/issue automatically (debugfs not available).")
	}
	return nil
}

func installApp(st *mockmender.State, imagePath string, metadata mockmender.AppMetaData) error {
	project := metadata.ApplicationName
	if project == "" {
		project = metadata.ProjectName
	}
	if project == "" {
		fmt.Println("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Missing application_name in artifact metadata\"")
		fmt.Println("Installation failed. System not modified.")
		return fmt.Errorf("missing application_name in artifact metadata")
	}
	if metadata.Orchestrator != "" && metadata.Orchestrator != "docker-compose" {
		fmt.Println("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Unsupported orchestrator\"")
		fmt.Println("Installation failed. System not modified.")
		return fmt.Errorf("unsupported orchestrator: %s", metadata.Orchestrator)
	}

	appPath := mockmender.AppPath(project)
	prevProject := project + "-previous"
	prevPath := mockmender.AppPath(prevProject)

	// Simulate docker compose down for previous rollout before rename.
	st.RunningContainers = mockmender.RemoveRunningContainersForProject(st.RunningContainers, project)
	if st.ErrorInjectPoint == mockmender.ErrInjectAfterStopOldContainers {
		// Keep the update in-progress after injected failure at this point.
		mockmender.SetInstalledApp(st, filepath.Base(imagePath), project, "", st.RunningContainers)
	}
	if err := maybeInjectedFailure(st, mockmender.ErrInjectAfterStopOldContainers); err != nil {
		return err
	}

	if _, err := os.Stat(appPath); err == nil {
		_ = os.RemoveAll(prevPath)
		if err := os.Rename(appPath, prevPath); err != nil {
			fmt.Printf("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
			fmt.Println("Installation failed. System not modified.")
			fmt.Printf("Could not fulfill request: %s\n", err.Error())
			return err
		}
		if err := maybeInjectedFailure(st, mockmender.ErrInjectAfterRenameOldAppDir); err != nil {
			return err
		}
	}

	manifestPath := mockmender.AppManifestPath(project)
	info, _, err := mockmender.ParseAndExtractAppArtifact(imagePath, manifestPath)
	if err != nil {
		_ = os.RemoveAll(appPath)
		if _, stErr := os.Stat(prevPath); stErr == nil {
			_ = os.Rename(prevPath, appPath)
		}
		fmt.Printf("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
		fmt.Println("Installation failed. System not modified.")
		fmt.Printf("Could not fulfill request: %s\n", err.Error())
		return err
	}
	if len(info.Payloads) == 0 || (info.Payloads[0].Type != string(mockmender.UpdateTypeApp) && info.Payloads[0].Type != "docker-compose") {
		fmt.Println("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Unsupported payload type\"")
		fmt.Println("Installation failed. System not modified.")
		return fmt.Errorf("unsupported payload")
	}
	if err := maybeInjectedFailure(st, mockmender.ErrInjectAfterExtractBeforeStart); err != nil {
		return err
	}

	running, err := mockmender.ComposeContainersFromManifest(appPath, project)
	if err != nil {
		fmt.Printf("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
		fmt.Println("Installation failed. System not modified.")
		fmt.Printf("Could not fulfill request: %s\n", err.Error())
		return err
	}
	st.RunningContainers = slices.Concat(st.RunningContainers, running)

	for i := 1; i <= 5; i++ {
		fmt.Printf("Progress: %d%%\n", i*20)
		time.Sleep(1 * time.Second)
	}

	if _, err := os.Stat(prevPath); err == nil {
		_ = os.RemoveAll(prevPath)
	}

	if err := mockmender.SaveState(*st); err != nil {
		return err
	}

	fmt.Println("Update Module doesn't support rollback. Committing immediately.")
	fmt.Println("Installed and committed.")
	return nil
}

func runCommit() error {
	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}

	idle, installed, trial := mockmender.Stage()
	switch st.Stage {
	case trial:
		if st.PendingUpdateType != string(mockmender.UpdateTypeRootfs) {
			fmt.Println("Nothing to commit.")
			return nil
		}
		mockmender.CommitTrial(&st)
		if err := mockmender.SaveState(st); err != nil {
			return err
		}
		fmt.Println("Committed.")
		return nil
	case installed:
		switch st.PendingUpdateType {
		case string(mockmender.UpdateTypeApp):
			pendingProject := st.PendingAppProject
			mockmender.CommitApp(&st)
			st.InconsistentApp = pendingProject
			if err := mockmender.SaveState(st); err != nil {
				return err
			}
			fmt.Println("Committed.")
			fmt.Println("Installation failed, and Update Module does not support rollback. System may be in an inconsistent state.")
			return nil
		default:
			mockmender.RollbackImmediate(&st)
			if err := mockmender.SaveState(st); err != nil {
				return err
			}
			fmt.Println("record_id=1 severity=info time=\"2026-Mar-03 07:38:33.551925\" name=\"Global\" msg=\"Update Module output (stderr): Mounted root does not match boot loader environment (/dev/mmcblk0p3)!\"")
			fmt.Println("record_id=2 severity=error time=\"2026-Mar-03 07:38:33.552810\" name=\"Global\" msg=\"Commit failed: Process returned non-zero exit status: ArtifactCommit: Process exited with status 1\"")
			fmt.Println("Installation failed. Rolled back modifications.")
			return fmt.Errorf("commit failed before reboot")
		}
	case idle:
		fmt.Println("Nothing to commit.")
		return nil
	default:
		fmt.Println("Nothing to commit.")
		return nil
	}
}

func runRollback() error {
	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}
	_, installed, trial := mockmender.Stage()
	switch st.Stage {
	case installed:
		mockmender.RollbackImmediate(&st)
	case trial:
		mockmender.RollbackAfterFailedTrial(&st)
	default:
	}
	if err := mockmender.SaveState(st); err != nil {
		return err
	}
	fmt.Println("Rolled back.")
	return nil
}

func runShowIssue() error {
	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}

	path := st.PendingIssuePath
	if path == "" {
		path = st.ActiveIssuePath
	}
	if path == "" {
		path = st.CommittedIssuePath
	}
	if path == "" {
		path = mockmender.IssueMirrorPath()
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fmt.Print(string(b))
	return nil
}

func runErrInject(point string) error {
	if !mockmender.IsValidErrInjectPoint(point) {
		return fmt.Errorf("invalid err-inject point: %s", point)
	}

	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}
	st.ErrorInjectPoint = point
	if err := mockmender.SaveState(st); err != nil {
		return err
	}
	if point == mockmender.ErrInjectNone {
		fmt.Println("Error injection cleared.")
	} else {
		fmt.Printf("Error injection set to: %s\n", point)
	}
	return nil
}

func maybeInjectedFailure(st *mockmender.State, point string) error {
	if st.ErrorInjectPoint != point {
		return nil
	}
	if point == mockmender.ErrInjectAfterExtractBeforeStart {
		if err := mockmender.RotateBootID(); err != nil {
			return err
		}
	}
	if err := mockmender.SaveState(*st); err != nil {
		return err
	}
	if mockmender.ShouldKillParent() {
		_ = mockmender.KillParentProcess()
	}
	return fmt.Errorf("injected error at %s", point)
}

func failIfAppStateInconsistent(st *mockmender.State, metadata mockmender.AppMetaData) error {
	project := metadata.ApplicationName
	if project == "" {
		project = metadata.ProjectName
	}
	if project == "" || st.InconsistentApp == "" || st.InconsistentApp != project {
		return nil
	}

	appPath := mockmender.AppPath(project)
	prevPath := mockmender.AppPath(project + "-previous")
	appExists, err := pathExists(appPath)
	if err != nil {
		return err
	}
	prevExists, err := pathExists(prevPath)
	if err != nil {
		return err
	}

	if appExists || prevExists {
		msg := "Installation failed, and Update Module does not support rollback. System may be in an inconsistent state."
		fmt.Println(msg)
		return errors.New(msg)
	}

	st.InconsistentApp = ""
	return mockmender.SaveState(*st)
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
