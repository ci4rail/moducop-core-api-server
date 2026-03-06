package mockmender

import (
	"os"
	"syscall"
)

const killParentEnv = "MOCK_MENDER_KILL_PARENT"

func ShouldKillParent() bool {
	return os.Getenv(killParentEnv) == "yes"
}

func KillParentProcess() error {
	parentPID := os.Getppid()
	parent, err := os.FindProcess(parentPID)
	if err != nil {
		return err
	}
	return parent.Signal(syscall.SIGKILL)
}
