package execcli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
)

func TestRunCommandWithLoggerStreamsOutput(t *testing.T) {
	var logBuf bytes.Buffer
	logger := loglite.New("exec-test", &logBuf, loglite.Info)

	stdout, stderr, exitCode, err := RunCommandWithLogger(
		"sh",
		5*time.Second,
		logger,
		"-c",
		"printf 'out-line\\nlast-out'; printf 'err-line\\nlast-err' >&2",
	)
	if err != nil {
		t.Fatalf("RunCommandWithLogger() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("RunCommandWithLogger() exitCode = %d, want 0", exitCode)
	}
	if stdout != "out-line\nlast-out" {
		t.Fatalf("stdout = %q, want %q", stdout, "out-line\nlast-out")
	}
	if stderr != "err-line\nlast-err" {
		t.Fatalf("stderr = %q, want %q", stderr, "err-line\nlast-err")
	}

	logged := logBuf.String()
	for _, want := range []string{
		"sh stdout: out-line",
		"sh stdout: last-out",
		"sh stderr: err-line",
		"sh stderr: last-err",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log output missing %q:\n%s", want, logged)
		}
	}
}
