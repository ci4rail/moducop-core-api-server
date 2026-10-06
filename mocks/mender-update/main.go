/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ci4rail/moducop-core-api-server/mocks/mockmender"
)

const expectedDeviceType = "moducop-cpu01"

var errNoUpdate = errors.New("no update in progress")

func menderMajorVersion() string {
	if v := os.Getenv("MOCK_MENDER_VERSION"); v != "" {
		return v
	}
	return "4"
}

func printFailure(operation, disposition string) {
	if menderMajorVersion() == "5" {
		fmt.Println(operation + " failed.")
		fmt.Println(disposition)
		return
	}
	switch disposition {
	case "Rolled back.":
		fmt.Println("Installation failed. Rolled back modifications.")
	case "Update Module does not support rollback. System may be in an inconsistent state.":
		fmt.Println("Installation failed, and Update Module does not support rollback. System may be in an inconsistent state.")
	default:
		fmt.Println("Installation failed. " + disposition)
	}
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, errNoUpdate) {
		return 2
	}
	return 1
}

func noUpdate() error {
	if menderMajorVersion() == "5" {
		fmt.Println("No update in progress.")
		return errNoUpdate
	}
	// Preserve the existing mock's behavior in the default profile.
	fmt.Println("Nothing to commit.")
	return nil
}

func main() {
	if v := menderMajorVersion(); v != "4" && v != "5" {
		fmt.Fprintln(os.Stderr, "MOCK_MENDER_VERSION must be 4 or 5")
		os.Exit(1)
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		if menderMajorVersion() == "5" {
			fmt.Println("5.1.0")
		} else {
			fmt.Println("4.0.5")
		}
		return
	}

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
			os.Exit(commandExitCode(err))
		}
	case "resume":
		if menderMajorVersion() != "5" {
			printRequestError("No such action: resume")
			os.Exit(1)
		}
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		if err := runResume(); err != nil {
			os.Exit(commandExitCode(err))
		}
	case "commit":
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		if err := runCommit(); err != nil {
			os.Exit(commandExitCode(err))
		}
	case "rollback":
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		if err := runRollback(); err != nil {
			os.Exit(commandExitCode(err))
		}
	case "show-issue":
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		if err := runShowIssue(); err != nil {
			os.Exit(commandExitCode(err))
		}
	case "err-inject":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		if err := runErrInject(os.Args[2]); err != nil {
			os.Exit(commandExitCode(err))
		}
	default:
		usage()
		os.Exit(2)
	}
}

func printRequestError(msg string) {
	if menderMajorVersion() == "5" {
		fmt.Fprintln(os.Stderr, "Could not fulfill request: "+msg)
	} else {
		fmt.Println("Could not fulfill request: " + msg)
	}
}

func printDiagnostic(format string, args ...any) {
	output := os.Stdout
	if menderMajorVersion() == "5" {
		output = os.Stderr
	}
	fmt.Fprintln(output, fmt.Sprintf(strings.TrimSuffix(format, "\n"), args...))
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  mender-update install <image-file>")
	fmt.Fprintln(os.Stderr, "  mender-update commit")
	if menderMajorVersion() == "5" {
		fmt.Fprintln(os.Stderr, "  mender-update resume")
	}
	fmt.Fprintln(os.Stderr, "  mender-update rollback")
	fmt.Fprintln(os.Stderr, "  mender-update show-issue")
	fmt.Fprintln(os.Stderr, "  mender-update err-inject <none|after-stop-old-containers|after-renaming-old-application-directory|after-extracting-new-application-before-starting-new-containers|docker-compose-up-failed>")
}

func runInstall(_ context.Context, imagePath string) error {
	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}
	_, installed, trial := mockmender.Stage()
	if _, err := os.Stat(imagePath); err != nil {
		name := filepath.Base(imagePath)
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"No such file or directory: Failed to open '%s' for reading\"\n", name)
		printFailure("Streaming", "System not modified.")
		printRequestError(fmt.Sprintf("No such file or directory: Failed to open '%s' for reading", name))
		return err
	}

	info, metadata, err := mockmender.ParseArtifactHeader(imagePath)
	if err != nil {
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
		printFailure("Streaming", "System not modified.")
		printRequestError(err.Error())
		return err
	}

	if len(info.ArtifactDepends.DeviceType) == 0 || info.ArtifactDepends.DeviceType[0] != expectedDeviceType {
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Artifact device type doesn't match\"")
		printFailure("Streaming", "System not modified.")
		return fmt.Errorf("device type mismatch")
	}
	if len(info.Payloads) == 0 {
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Unsupported payload type\"")
		printFailure("Streaming", "System not modified.")
		return fmt.Errorf("unsupported payload")
	}

	switch info.Payloads[0].Type {
	case string(mockmender.UpdateTypeRootfs):
		if st.Stage == installed || st.Stage == trial {
			msg := "Operation now in progress: Update already in progress. Please commit or roll back first"
			printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:09:26.999642\" name=\"Global\" msg=\"%s\"\n", msg)
			if menderMajorVersion() == "4" {
				printFailure("Streaming", "System not modified.")
			}
			printRequestError(msg)
			return errors.New(msg)
		}
		st.PendingUpdateType = string(mockmender.UpdateTypeRootfs)
		if err := checkpoint(&st, imagePath, mockmender.PhaseInstalling); err != nil {
			return err
		}
		return installRootfs(&st, imagePath)
	case string(mockmender.UpdateTypeApp), "docker-compose":
		if err := failIfAppStateInconsistent(&st, metadata); err != nil {
			return err
		}
		if st.Stage == installed || st.Stage == trial {
			msg := "Operation now in progress: Update already in progress. Please commit or roll back first"
			if menderMajorVersion() == "4" {
				printFailure("Streaming", "System not modified.")
			}
			printRequestError(msg)
			return errors.New(msg)
		}
		return installApp(&st, imagePath, metadata)
	case string(mockmender.UpdateTypeCustomization):
		if st.Stage == installed || st.Stage == trial {
			msg := "Operation now in progress: Update already in progress. Please commit or roll back first"
			if menderMajorVersion() == "4" {
				printFailure("Streaming", "System not modified.")
			}
			printRequestError(msg)
			return errors.New(msg)
		}
		st.PendingUpdateType = string(mockmender.UpdateTypeCustomization)
		if err := checkpoint(&st, imagePath, mockmender.PhaseInstalling); err != nil {
			return err
		}
		return installCustomization(&st, imagePath)
	default:
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Unsupported payload type\"")
		printFailure("Streaming", "System not modified.")
		return fmt.Errorf("unsupported payload")
	}
}

func installCustomization(st *mockmender.State, imagePath string) error {
	info, manifest, healthExpectedPass, err := mockmender.ParseCustomizationArtifact(imagePath)
	if err != nil {
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
		printFailure("Installation", "System not modified.")
		printRequestError(err.Error())
		return err
	}
	if len(info.Payloads) == 0 || info.Payloads[0].Type != string(mockmender.UpdateTypeCustomization) {
		return fmt.Errorf("unsupported payload type")
	}
	mockmender.SetInstalledCustomization(st, filepath.Base(imagePath), manifest.Version, healthExpectedPass)
	if err := checkpoint(st, imagePath, mockmender.PhaseAwaitingCommit); err != nil {
		return err
	}
	if err := mockmender.SaveState(*st); err != nil {
		return err
	}
	fmt.Println("Installed, but not committed.")
	fmt.Println("Use 'commit' to update, or 'rollback' to roll back the update.")
	return nil
}

func installRootfs(st *mockmender.State, imagePath string) error {
	info, extractedRootfs, err := mockmender.ParseAndExtractArtifact(imagePath)
	if err != nil {
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
		printFailure("Installation", "System not modified.")
		printRequestError(err.Error())
		return err
	}
	if len(info.Payloads) == 0 || info.Payloads[0].Type != string(mockmender.UpdateTypeRootfs) {
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Unsupported payload type\"")
		printFailure("Installation", "System not modified.")
		return fmt.Errorf("unsupported payload")
	}

	for i := 1; i <= 10; i++ {
		fmt.Printf("Progress: %d%%\n", i*10)
		time.Sleep(1 * time.Second)
	}

	ext4ImagePath, issuePath, err := mockmender.PrepareRootfsInspection(extractedRootfs)
	if err != nil {
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
		printFailure("Installation", "System not modified.")
		printRequestError(err.Error())
		return err
	}
	mockmender.SetInstalledRootfs(st, filepath.Base(imagePath), extractedRootfs, ext4ImagePath, issuePath)
	if err := checkpoint(st, imagePath, mockmender.PhaseAwaitingCommit); err != nil {
		return err
	}
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
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Missing application_name in artifact metadata\"")
		printFailure("Installation", "System not modified.")
		return fmt.Errorf("missing application_name in artifact metadata")
	}
	if metadata.Orchestrator != "" && metadata.Orchestrator != "docker-compose" {
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Unsupported orchestrator\"")
		printFailure("Installation", "System not modified.")
		return fmt.Errorf("unsupported orchestrator: %s", metadata.Orchestrator)
	}

	if menderMajorVersion() == "5" && st.InstallPhase == "" {
		mockmender.SetInstalledApp(st, filepath.Base(imagePath), project, "", st.RunningContainers)
		if err := checkpoint(st, imagePath, mockmender.PhaseInstalling); err != nil {
			return err
		}
	}
	appPath := mockmender.AppPath(project)
	prevProject := project + "-previous"
	prevPath := mockmender.AppPath(prevProject)

	if st.InstallPhase != mockmender.PhaseAppStopped && st.InstallPhase != mockmender.PhaseAppRenamed && st.InstallPhase != mockmender.PhaseAppExtracted {
		// Simulate docker compose down for previous rollout before rename.
		st.RunningContainers = mockmender.RemoveRunningContainersForProject(st.RunningContainers, project)
		if st.ErrorInjectPoint == mockmender.ErrInjectAfterStopOldContainers {
			// Keep the update in-progress after injected failure at this point.
			mockmender.SetInstalledApp(st, filepath.Base(imagePath), project, "", st.RunningContainers)
		}
		if err := checkpoint(st, imagePath, mockmender.PhaseAppStopped); err != nil {
			return err
		}
		if err := maybeInjectedFailure(st, mockmender.ErrInjectAfterStopOldContainers); err != nil {
			return err
		}
	}
	if st.InstallPhase != mockmender.PhaseAppRenamed && st.InstallPhase != mockmender.PhaseAppExtracted {
		if _, err := os.Stat(appPath); err == nil {
			_ = os.RemoveAll(prevPath)
			if err := os.Rename(appPath, prevPath); err != nil {
				printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
				printFailure("Installation", "System not modified.")
				printRequestError(err.Error())
				return err
			}
			if menderMajorVersion() == "5" {
				st.PreviousAppProject = prevProject
			}
			if err := checkpoint(st, imagePath, mockmender.PhaseAppRenamed); err != nil {
				return err
			}
			if err := maybeInjectedFailure(st, mockmender.ErrInjectAfterRenameOldAppDir); err != nil {
				return err
			}
		}
	}
	if st.InstallPhase != mockmender.PhaseAppExtracted {
		manifestPath := mockmender.AppManifestPath(project)
		info, _, err := mockmender.ParseAndExtractAppArtifact(imagePath, manifestPath)
		if err != nil {
			_ = os.RemoveAll(appPath)
			if _, stErr := os.Stat(prevPath); stErr == nil {
				_ = os.Rename(prevPath, appPath)
			}
			printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
			printFailure("Installation", "System not modified.")
			printRequestError(err.Error())
			return err
		}
		if len(info.Payloads) == 0 || (info.Payloads[0].Type != string(mockmender.UpdateTypeApp) && info.Payloads[0].Type != "docker-compose") {
			printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:43:32.990506\" name=\"Global\" msg=\"Unsupported payload type\"")
			printFailure("Installation", "System not modified.")
			return fmt.Errorf("unsupported payload")
		}
		if st.ErrorInjectPoint == mockmender.ErrInjectDockerComposeUpFailed {
			// docker compose down has already stopped the old rollout. Preserve that
			// side effect even though the subsequent compose up fails.
			if err := mockmender.SaveState(*st); err != nil {
				return err
			}
			fmt.Println("Error response from daemon: invalid mount config for type \"bind\": bind source path does not exist: /data/missing-bind-source")
			fmt.Println("unsuccessful rollout")
			printFailure("Installation", "Update Module does not support rollback. System may be in an inconsistent state.")
			return errors.New("docker compose up failed because bind source path does not exist")
		}
		if err := checkpoint(st, imagePath, mockmender.PhaseAppExtracted); err != nil {
			return err
		}
		if err := maybeInjectedFailure(st, mockmender.ErrInjectAfterExtractBeforeStart); err != nil {
			return err
		}

	}

	running, err := mockmender.ComposeContainersFromManifest(appPath, project)
	if err != nil {
		printDiagnostic("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"%s\"\n", err.Error())
		printFailure("Installation", "System not modified.")
		printRequestError(err.Error())
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

	if menderMajorVersion() == "5" {
		mockmender.CommitApp(st)
	}
	if err := mockmender.SaveState(*st); err != nil {
		return err
	}

	fmt.Println("Update Module doesn't support rollback. Committing immediately.")
	fmt.Println("Installed and committed.")
	return nil
}

func awaitingCommit(phase string) bool {
	return phase == mockmender.PhaseAwaitingCommit || phase == mockmender.PhaseCommitting
}

func runResume() error {
	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}
	idle, _, _ := mockmender.Stage()
	if st.Stage == idle {
		return noUpdate()
	}
	if st.InstallPhase == "" || awaitingCommit(st.InstallPhase) {
		return runCommit()
	}
	switch st.InstallPhase {
	case mockmender.PhaseInstalling, mockmender.PhaseAppStopped, mockmender.PhaseAppRenamed, mockmender.PhaseAppExtracted:
	default:
		err := errors.New("cannot resume unknown installation phase: " + st.InstallPhase)
		printRequestError(err.Error())
		return err
	}
	if st.ResumeArtifact == "" {
		err := errors.New("interrupted update has no saved artifact path")
		printRequestError(err.Error())
		return err
	}
	switch mockmender.UpdateType(st.PendingUpdateType) {
	case mockmender.UpdateTypeApp:
		_, metadata, err := mockmender.ParseArtifactHeader(st.ResumeArtifact)
		if err != nil {
			printRequestError(err.Error())
			return err
		}
		return installApp(&st, st.ResumeArtifact, metadata)
	case mockmender.UpdateTypeRootfs:
		return installRootfs(&st, st.ResumeArtifact)
	case mockmender.UpdateTypeCustomization:
		return installCustomization(&st, st.ResumeArtifact)
	default:
		err := errors.New("cannot resume unknown update type")
		printRequestError(err.Error())
		return err
	}
}

func checkpoint(st *mockmender.State, imagePath, phase string) error {
	if menderMajorVersion() != "5" {
		return nil
	}
	path, err := filepath.Abs(imagePath)
	if err != nil {
		return err
	}
	st.ResumeArtifact, st.InstallPhase = path, phase
	_, installed, _ := mockmender.Stage()
	st.Stage = installed
	return mockmender.SaveState(*st)
}

func runCommit() error {
	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}

	idle, installed, trial := mockmender.Stage()
	if menderMajorVersion() == "5" && st.Stage != idle &&
		((st.InstallPhase != "" && !awaitingCommit(st.InstallPhase)) ||
			(st.InstallPhase == "" && st.PendingUpdateType == string(mockmender.UpdateTypeApp))) {
		err := errors.New("Cannot commit from this state. Make sure that the `install` command has run successfully and the device is expecting a commit.")
		printRequestError(err.Error())
		return err
	}
	switch st.Stage {
	case trial:
		if st.PendingUpdateType != string(mockmender.UpdateTypeRootfs) {
			return noUpdate()
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
			if menderMajorVersion() == "5" && awaitingCommit(st.InstallPhase) {
				mockmender.CommitApp(&st)
				if err := mockmender.SaveState(st); err != nil {
					return err
				}
				fmt.Println("Committed.")
				return nil
			}
			pendingProject := st.PendingAppProject
			mockmender.CommitApp(&st)
			st.InconsistentApp = pendingProject
			if err := mockmender.SaveState(st); err != nil {
				return err
			}
			if menderMajorVersion() == "5" {
				printFailure("Committing", "Update Module does not support rollback. System may be in an inconsistent state.")
				return errors.New("commit failed for interrupted application update")
			}
			fmt.Println("Committed.")
			printFailure("Installation", "Update Module does not support rollback. System may be in an inconsistent state.")
			return nil
		case string(mockmender.UpdateTypeCustomization):
			if !st.CustomizationHealthEvaluated || !st.CustomizationHealthPassed {
				printFailure("Committing", "Rolled back.")
				return errors.New("customization health checks did not pass")
			}
			mockmender.CommitCustomization(&st)
			if err := mockmender.SaveState(st); err != nil {
				return err
			}
			fmt.Println("Committed.")
			return nil
		default:
			mockmender.RollbackImmediate(&st)
			if err := mockmender.SaveState(st); err != nil {
				return err
			}
			printDiagnostic("record_id=1 severity=info time=\"2026-Mar-03 07:38:33.551925\" name=\"Global\" msg=\"Update Module output (stderr): Mounted root does not match boot loader environment (/dev/mmcblk0p3)!\"")
			printDiagnostic("record_id=2 severity=error time=\"2026-Mar-03 07:38:33.552810\" name=\"Global\" msg=\"Commit failed: Process returned non-zero exit status: ArtifactCommit: Process exited with status 1\"")
			printFailure("Committing", "Rolled back.")
			return fmt.Errorf("commit failed before reboot")
		}
	case idle:
		return noUpdate()
	default:
		return noUpdate()
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
		if menderMajorVersion() == "5" {
			return noUpdate()
		}
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
		printFailure("Installation", "Update Module does not support rollback. System may be in an inconsistent state.")
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
