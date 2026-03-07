package diskstate

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	errCountMustBeNonNegative staticError = "count must be >= 0"
	errCountMustBeMaxTen      staticError = "count must be <= 10"
)

type testState struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	s := New[testState](path)

	in := testState{Name: "alpha", Count: 42}
	if err := s.Save(context.Background(), in); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var out testState
	if err := s.Load(context.Background(), &out); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if out != in {
		t.Fatalf("Load() got %+v, want %+v", out, in)
	}
}

func TestStoreLoadMissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.json")
	s := New[testState](path)

	var out testState
	err := s.Load(context.Background(), &out)
	if !stderrors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() error = %v, want os.ErrNotExist", err)
	}
}

func TestStoreLoadRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"name":"alpha","count":1,"extra":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s := New[testState](path)
	var out testState
	err := s.Load(context.Background(), &out)
	if err == nil {
		t.Fatalf("Load() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load() error = %v, want unknown field error", err)
	}
}

func TestStoreLoadRejectsTrailingJSONValue(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"name":"alpha","count":1}{"name":"beta","count":2}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s := New[testState](path)
	var out testState
	err := s.Load(context.Background(), &out)
	if err == nil {
		t.Fatalf("Load() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unexpected extra JSON value") {
		t.Fatalf("Load() error = %v, want unexpected extra JSON value", err)
	}
}

func TestStoreSaveValidationFailureDoesNotWriteFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	s := New[testState](path)
	s.Validate = func(v *testState) error {
		if v.Count < 0 {
			return errCountMustBeNonNegative
		}
		return nil
	}

	err := s.Save(context.Background(), testState{Name: "bad", Count: -1})
	if err == nil {
		t.Fatalf("Save() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "validate before save") {
		t.Fatalf("Save() error = %v, want validate before save prefix", err)
	}

	_, statErr := os.Stat(path)
	if !stderrors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("state file should not exist; Stat() error = %v", statErr)
	}
}

func TestStoreUpdateInitializesAndPersists(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	s := New[testState](path)

	if err := s.Update(context.Background(), func(v *testState) error {
		v.Name = "created"
		v.Count = 7
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	var out testState
	if err := s.Load(context.Background(), &out); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := testState{Name: "created", Count: 7}
	if out != want {
		t.Fatalf("Load() got %+v, want %+v", out, want)
	}
}

func TestStoreUpdateValidationFailureKeepsPreviousState(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	s := New[testState](path)
	s.Validate = func(v *testState) error {
		if v.Count > 10 {
			return errCountMustBeMaxTen
		}
		return nil
	}

	initial := testState{Name: "ok", Count: 5}
	if err := s.Save(context.Background(), initial); err != nil {
		t.Fatalf("Save() initial error = %v", err)
	}

	err := s.Update(context.Background(), func(v *testState) error {
		v.Count = 99
		return nil
	})
	if err == nil {
		t.Fatalf("Update() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "validate before save") {
		t.Fatalf("Update() error = %v, want validate before save prefix", err)
	}

	var out testState
	if err := s.Load(context.Background(), &out); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if out != initial {
		t.Fatalf("state changed after failed update: got %+v, want %+v", out, initial)
	}
}
