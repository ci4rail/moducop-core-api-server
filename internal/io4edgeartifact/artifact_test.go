package io4edgeartifact

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetManifestFromFile(t *testing.T) {
	t.Parallel()

	firmwarePath := filepath.Join("..", "..", "tests", "assets", "fw-iou16-00-default-1.0.2.fwpkg")

	manifest, err := GetManifestFromFile(firmwarePath)
	if err != nil {
		t.Fatalf("GetManifestFromFile() error = %v", err)
	}

	if manifest.Name != "iou16-00-default" {
		t.Fatalf("unexpected manifest name: got %q, want %q", manifest.Name, "iou16-00-default")
	}
	if manifest.Version != "1.0.2" {
		t.Fatalf("unexpected manifest version: got %q, want %q", manifest.Version, "1.0.2")
	}
	if manifest.File != "fw-iou16-00-default.bin" {
		t.Fatalf("unexpected manifest file: got %q, want %q", manifest.File, "fw-iou16-00-default.bin")
	}
}

func TestGetManifestFromFileErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		entries map[string]string
		wantErr string
	}{
		{
			name: "missing manifest",
			entries: map[string]string{
				"fw.bin": "payload",
			},
			wantErr: "invalid firmware package: missing manifest.json",
		},
		{
			name: "invalid manifest json",
			entries: map[string]string{
				"manifest.json": "{",
			},
			wantErr: "invalid firmware manifest:",
		},
		{
			name: "missing firmware binary referenced in manifest",
			entries: map[string]string{
				"manifest.json": `{"name":"iou16-00-default","version":"1.0.2","file":"fw.bin"}`,
			},
			wantErr: "invalid firmware package: missing firmware binary",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			firmwarePath := createFirmwarePackage(t, tc.entries)
			_, err := GetManifestFromFile(firmwarePath)
			if err == nil {
				t.Fatalf("expected error but got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: got %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func createFirmwarePackage(t *testing.T, entries map[string]string) string {
	t.Helper()

	firmwarePath := filepath.Join(t.TempDir(), "test.fwpkg")
	f, err := os.Create(firmwarePath)
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	tw := tar.NewWriter(f)
	for name, content := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name: "./" + name,
			Mode: 0o644,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatalf("WriteHeader() error = %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close error = %v", err)
	}

	return firmwarePath
}
