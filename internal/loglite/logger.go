/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

// loglite is a simple logger wrapper around log.Logger with levels, timestamps, and caller info.
package loglite

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

type Level uint32

const (
	Debug Level = iota
	Info
	Warn
	Error
	Off
)

func (l Level) String() string {
	switch l {
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	case Off:
		return "OFF"
	default:
		return "UNKNOWN"
	}
}

type Logger struct {
	name   string
	l      *log.Logger
	minLvl atomic.Uint32
	clock  func() time.Time
}

const callerSkip = 4

// New creates a named logger writing to w.
// Timestamp is included by this wrapper (so the underlying log.Logger uses flags=0).
func New(name string, w io.Writer, minLevel Level) *Logger {
	ll := log.New(w, "", 0) // we format everything ourselves
	lg := &Logger{
		name:  name,
		l:     ll,
		clock: time.Now,
	}
	lg.minLvl.Store(uint32(minLevel))
	return lg
}

func (lg *Logger) SetLevel(minLevel Level) { lg.minLvl.Store(uint32(minLevel)) }
func (lg *Logger) Level() Level            { return Level(lg.minLvl.Load()) }
func (lg *Logger) Name() string            { return lg.name }

// Debugf/Infof/Warnf/Errorf
func (lg *Logger) Debugf(format string, args ...any) { lg.printf(Debug, format, args...) }
func (lg *Logger) Infof(format string, args ...any)  { lg.printf(Info, format, args...) }
func (lg *Logger) Warnf(format string, args ...any)  { lg.printf(Warn, format, args...) }
func (lg *Logger) Errorf(format string, args ...any) { lg.printf(Error, format, args...) }

// Also handy non-format variants:
func (lg *Logger) Debug(args ...any) { lg.print(Debug, args...) }
func (lg *Logger) Info(args ...any)  { lg.print(Info, args...) }
func (lg *Logger) Warn(args ...any)  { lg.print(Warn, args...) }
func (lg *Logger) Error(args ...any) { lg.print(Error, args...) }

func (lg *Logger) Enabled(level Level) bool {
	minLevel := Level(lg.minLvl.Load())
	return level >= minLevel && minLevel != Off
}

func (lg *Logger) printf(level Level, format string, args ...any) {
	if !lg.Enabled(level) {
		return
	}
	msg := fmt.Sprintf(format, args...)
	lg.output(level, msg, callerSkip) // caller skip
}

func (lg *Logger) print(level Level, args ...any) {
	if !lg.Enabled(level) {
		return
	}
	msg := fmt.Sprint(args...)
	lg.output(level, msg, callerSkip) // caller skip
}

func (lg *Logger) output(level Level, msg string, skip int) {
	ts := lg.clock().Format("2006-01-02T15:04:05.000Z07:00")

	fileLine := callerFileLine(skip)

	// Ensure exactly one line per call; if msg contains newlines, indent continuation lines.
	msg = strings.TrimRight(msg, "\n")
	if strings.Contains(msg, "\n") {
		msg = indentMultiline(msg, "    ")
	}

	// Example:
	// 2026-03-03T12:34:56.123456789+01:00 INFO  mysvc main.go:42 hello
	line := fmt.Sprintf("%s %-5s %-12s %s %s", ts, level.String(), lg.name, fileLine, msg)
	lg.l.Print(line)
}

func callerFileLine(skip int) string {
	// skip is relative to this helper; we add +1 to get the caller of callerFileLine.
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "?:0"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

func indentMultiline(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}
