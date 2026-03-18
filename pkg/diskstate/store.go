/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package diskstate

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Store persists a value of type T to a single file path.
// It uses a crash-safe write pattern: write temp -> fsync -> rename -> fsync dir.
type Store[T any] struct {
	path string
	mu   sync.Mutex

	// Optional hooks
	Validate func(v *T) error // called after load (and before save)
}

const fileModeOwnerRW = 0o600
const bitsPerUint32 = 32

type staticError string

func (e staticError) Error() string { return string(e) }

const errUnexpectedExtraJSONValue staticError = "unexpected extra JSON value"

// New creates a new Store using the given target file path.
// The directory must exist (or you can create it before calling New).
func New[T any](path string) *Store[T] {
	return &Store[T]{path: path}
}

// Path returns the target file path.
func (s *Store[T]) Path() string { return s.path }

// Load reads the latest committed state into *out.
// If the file does not exist, it returns os.ErrNotExist.
func (s *Store[T]) Load(ctx context.Context, out *T) error {
	_ = ctx // reserved for future (e.g., cancellation around slow IO)
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields() // change if you want schema evolution tolerance
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", s.path, err)
	}

	// Ensure there's no trailing junk.
	if err := ensureEOF(dec); err != nil {
		return fmt.Errorf("decode %s: %w", s.path, err)
	}

	if s.Validate != nil {
		if err := s.Validate(out); err != nil {
			return fmt.Errorf("validate %s: %w", s.path, err)
		}
	}
	return nil
}

// Save atomically commits v to disk.
// It will never leave a partially-written target file behind.
func (s *Store[T]) Save(ctx context.Context, v T) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Validate != nil {
		tmp := v
		if err := s.Validate(&tmp); err != nil {
			return fmt.Errorf("validate before save: %w", err)
		}
	}

	return s.saveUnlocked(v)
}

// Update loads (or initializes) the state, runs fn under the store lock,
// then commits the result. If the file doesn't exist, it starts from zero value.
func (s *Store[T]) Update(ctx context.Context, fn func(*T) error) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	var cur T
	if err := s.loadUnlocked(&cur); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// zero value is fine
	}

	if err := fn(&cur); err != nil {
		return err
	}

	if s.Validate != nil {
		if err := s.Validate(&cur); err != nil {
			return fmt.Errorf("validate before save: %w", err)
		}
	}

	return s.saveUnlocked(cur)
}

func (s *Store[T]) loadUnlocked(out *T) error {
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", s.path, err)
	}
	if err := ensureEOF(dec); err != nil {
		return fmt.Errorf("decode %s: %w", s.path, err)
	}
	if s.Validate != nil {
		if err := s.Validate(out); err != nil {
			return fmt.Errorf("validate %s: %w", s.path, err)
		}
	}
	return nil
}

func (s *Store[T]) saveUnlocked(v T) error {
	dir := filepath.Dir(s.path)
	base := filepath.Base(s.path)

	tmpPath := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%d.%d", base, os.Getpid(), randSuffix()))
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileModeOwnerRW)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}

	cleanup := func(err error) error {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return cleanup(fmt.Errorf("encode temp: %w", err))
	}
	if err := f.Sync(); err != nil {
		return cleanup(fmt.Errorf("fsync temp: %w", err))
	}
	if err := f.Close(); err != nil {
		return cleanup(fmt.Errorf("close temp: %w", err))
	}

	if runtime.GOOS == "windows" {
		if err := replaceFileWindows(tmpPath, s.path); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
	} else {
		if err := os.Rename(tmpPath, s.path); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("rename into place: %w", err)
		}
	}

	if err := syncDir(dir); err != nil {
		return fmt.Errorf("fsync dir: %w", err)
	}
	return nil
}

func ensureEOF(dec *json.Decoder) error {
	// After decoding exactly one JSON value, the stream should be at EOF
	// (ignoring trailing whitespace).
	var extra any
	if err := dec.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		// e.g. syntax error in trailing junk
		return err
	}
	return errUnexpectedExtraJSONValue
}

func randSuffix() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		high := int64(binary.LittleEndian.Uint32(b[:4]))
		low := int64(binary.LittleEndian.Uint32(b[4:]))
		return (high << bitsPerUint32) | low
	}
	return int64(os.Getpid())
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// replaceFileWindows replaces dst with src atomically-ish on Windows.
// Windows rename does not replace existing files, so we:
// 1) rename dst -> bak
// 2) rename src -> dst
// 3) remove bak
//
// Power-loss behavior: worst case you may see dst missing but bak present.
// If you need stronger guarantees on Windows, we can add recovery logic (.bak)
// on Load, but many embedded targets are Linux.
func replaceFileWindows(src, dst string) error {
	dir := filepath.Dir(dst)
	base := filepath.Base(dst)
	bak := filepath.Join(dir, fmt.Sprintf(".%s.bak.%d", base, os.Getpid()))

	// If dst exists, move it aside.
	if err := os.Rename(dst, bak); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			// if dst didn't exist, that's fine
			// but Windows often returns a PathError; check via Stat
			if _, statErr := os.Stat(dst); statErr == nil {
				return fmt.Errorf("rename old -> bak: %w", err)
			}
		}
	}

	// Move new into place.
	if err := os.Rename(src, dst); err != nil {
		// Try to restore old.
		_ = os.Rename(bak, dst)
		return fmt.Errorf("rename new -> dst: %w", err)
	}

	// Best-effort cleanup.
	_ = os.Remove(bak)

	// fsync directory for the dst entry.
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("fsync dir: %w", err)
	}
	return nil
}
