/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package execcli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
)

const streamCount = 2

// RunCommand executes the given command with arguments and returns its stdout, stderr, exit code and error (if any).
func RunCommand(cmd string, timeout time.Duration, args ...string) (stdout string, stderr string, exitCode int, err error) {
	return RunCommandWithLogger(cmd, timeout, nil, args...)
}

// RunCommandWithLogger executes the given command with arguments, streams process output
// to the provided logger in real time, and returns stdout, stderr, exit code and error (if any).
func RunCommandWithLogger(cmd string, timeout time.Duration, logger *loglite.Logger, args ...string) (stdout string, stderr string, exitCode int, err error) {
	c := exec.CommandContext(context.Background(), cmd, args...)
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
	stdoutLogger := newLogWriter(logger, cmd, "stdout")
	stderrLogger := newLogWriter(logger, cmd, "stderr")
	var wg sync.WaitGroup
	wg.Add(streamCount)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.MultiWriter(&outBuf, stdoutLogger), stdoutPipe)
		flushLogWriter(stdoutLogger)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.MultiWriter(&errBuf, stderrLogger), stderrPipe)
		flushLogWriter(stderrLogger)
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

type logWriter struct {
	logger *loglite.Logger
	cmd    string
	stream string
	mu     sync.Mutex
	buf    strings.Builder
}

func newLogWriter(logger *loglite.Logger, cmd, stream string) io.Writer {
	if logger == nil {
		return io.Discard
	}
	return &logWriter{
		logger: logger,
		cmd:    cmd,
		stream: stream,
	}
}

func flushLogWriter(w io.Writer) {
	lw, ok := w.(*logWriter)
	if !ok {
		return
	}
	lw.mu.Lock()
	defer lw.mu.Unlock()
	lw.flushLocked()
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	scanner := bufio.NewScanner(bytes.NewReader(p))
	scanner.Split(scanLinesWithTrailingFragment)
	for scanner.Scan() {
		part := scanner.Text()
		if strings.HasSuffix(part, "\n") {
			w.buf.WriteString(strings.TrimSuffix(part, "\n"))
			w.flushLocked()
			continue
		}
		w.buf.WriteString(part)
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *logWriter) flushLocked() {
	if w.buf.Len() == 0 {
		return
	}
	msg := w.buf.String()
	w.buf.Reset()
	w.logger.Infof("%s %s: %s", w.cmd, w.stream, msg)
}

func scanLinesWithTrailingFragment(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
