package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
}

func runInstall(_ context.Context, imagePath string) error {
	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}
	_, installed, trial := mockmender.Stage()
	if st.Stage == installed || st.Stage == trial {
		msg := "Operation now in progress: Update already in progress. Please commit or roll back first"
		fmt.Printf("record_id=1 severity=error time=\"2026-Mar-03 07:09:26.999642\" name=\"Global\" msg=\"%s\"\n", msg)
		fmt.Println("Installation failed. System not modified.")
		fmt.Printf("Could not fulfill request: %s\n", msg)
		return fmt.Errorf(msg)
	}

	if _, err := os.Stat(imagePath); err != nil {
		name := filepath.Base(imagePath)
		fmt.Printf("record_id=1 severity=error time=\"2026-Mar-03 07:05:21.952463\" name=\"Global\" msg=\"No such file or directory: Failed to open '%s' for reading\"\n", name)
		fmt.Println("Installation failed. System not modified.")
		fmt.Printf("Could not fulfill request: No such file or directory: Failed to open '%s' for reading\n", name)
		return err
	}

	info, extractedRootfs, err := mockmender.ParseAndExtractArtifact(imagePath)
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
	if len(info.Payloads) == 0 || info.Payloads[0].Type != "rootfs-image" {
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
	mockmender.SetInstalled(&st, filepath.Base(imagePath), extractedRootfs, ext4ImagePath, issuePath)
	if err := mockmender.SaveState(st); err != nil {
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

func runCommit() error {
	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}

	idle, installed, trial := mockmender.Stage()
	switch st.Stage {
	case trial:
		mockmender.CommitTrial(&st)
		if err := mockmender.SaveState(st); err != nil {
			return err
		}
		fmt.Println("Committed.")
		return nil
	case installed:
		mockmender.RollbackImmediate(&st)
		if err := mockmender.SaveState(st); err != nil {
			return err
		}
		fmt.Println("record_id=1 severity=info time=\"2026-Mar-03 07:38:33.551925\" name=\"Global\" msg=\"Update Module output (stderr): Mounted root does not match boot loader environment (/dev/mmcblk0p3)!\"")
		fmt.Println("record_id=2 severity=error time=\"2026-Mar-03 07:38:33.552810\" name=\"Global\" msg=\"Commit failed: Process returned non-zero exit status: ArtifactCommit: Process exited with status 1\"")
		fmt.Println("Installation failed. Rolled back modifications.")
		return fmt.Errorf("commit failed before reboot")
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
		path = st.CommittedIssuePath
	}
	if path == "" {
		return fmt.Errorf("no extracted /etc/issue available")
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fmt.Print(string(b))
	return nil
}
