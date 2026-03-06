package execcli

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// RunCommand executes the given command with arguments and returns its stdout, stderr, exit code and error (if any).
func RunCommand(cmd string, timeout time.Duration, args ...string) (stdout string, stderr string, exitCode int, err error) {
	c := exec.Command(cmd, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // separate process group

	stdoutPipe, err := c.StdoutPipe()
	if err != nil {
		return "", "", -1, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := c.StderrPipe()
	if err != nil {
		return "", "", -1, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := c.Start(); err != nil {
		return "", "", -1, fmt.Errorf("start command: %w", err)
	}

	var outBuf, errBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(&outBuf, stdoutPipe)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&errBuf, stderrPipe)
	}()

	waitCh := make(chan error, 1)
	go func() { waitCh <- c.Wait() }()

	select {
	case waitErr := <-waitCh:
		wg.Wait()
		stdout, stderr = outBuf.String(), errBuf.String()
		exitCode = c.ProcessState.ExitCode()
		if waitErr != nil {
			return stdout, stderr, exitCode, fmt.Errorf("command failed: %w", waitErr)
		}
		return stdout, stderr, exitCode, nil

	case <-time.After(timeout):
		// kill whole process group
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		waitErr := <-waitCh
		wg.Wait()
		stdout, stderr = outBuf.String(), errBuf.String()
		if c.ProcessState != nil {
			exitCode = c.ProcessState.ExitCode()
		} else {
			exitCode = -1
		}
		log.Printf("command timed out after %s", timeout)
		return stdout, stderr, exitCode, fmt.Errorf("timeout after %s: %w", timeout, waitErr)
	}
}
