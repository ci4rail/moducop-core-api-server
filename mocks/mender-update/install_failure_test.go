/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ci4rail/moducop-core-api-server/mocks/mockmender"
)

func failureTestTar(t *testing.T, name, content string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func failureTestArtifact(t *testing.T, payloadType, manifest string) string {
	t.Helper()
	header := failureTestTar(t, "header-info", fmt.Sprintf("{\"payloads\":[{\"type\":%q}],\"artifact_depends\":{\"device_type\":[\"moducop-cpu01\"]}}", payloadType))
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, entry := range []struct {
		name string
		data []byte
	}{
		{"header.tar", header},
		{"data/0000.tar", failureTestTar(t, "manifest.json", manifest)},
	} {
		if err := w.WriteHeader(&tar.Header{Name: entry.name, Mode: 0600, Size: int64(len(entry.data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "update.mender")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFailedNonApplicationInstallClearsCheckpoint(t *testing.T) {
	for _, payloadType := range []string{"rootfs-image", "os-customization"} {
		for _, resume := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/resume=%v", payloadType, resume), func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("MOCK_MENDER_STATE_DIR", root)
				t.Setenv("MOCK_MENDER_FS_ROOT", filepath.Join(root, "fs"))
				st, err := mockmender.LoadState()
				if err != nil {
					t.Fatal(err)
				}
				st.ActiveCustomizationVersion = "previous"
				if err := mockmender.SaveState(st); err != nil {
					t.Fatal(err)
				}
				bad := failureTestArtifact(t, payloadType, "invalid json")
				if resume {
					st.PendingUpdateType = payloadType
					if err := checkpoint(&st, bad, mockmender.PhaseInstalling); err != nil {
						t.Fatal(err)
					}
					err = runResume(true)
				} else {
					err = runInstall(context.Background(), bad)
				}
				if err == nil {
					t.Fatal("invalid payload succeeded")
				}
				if err := os.Remove(bad); err != nil {
					t.Fatal(err)
				}
				after, err := mockmender.LoadState()
				if err != nil {
					t.Fatal(err)
				}
				idle, _, _ := mockmender.Stage()
				if after.Stage != idle || after.InstallPhase != "" || after.ResumeArtifact != "" || after.PendingUpdateType != "" {
					t.Fatalf("failed install left transaction: %+v", after)
				}
				if after.ActiveCustomizationVersion != "previous" {
					t.Fatal("failed install changed active customization")
				}
				good := failureTestArtifact(t, "os-customization", "{\"version\":\"next\"}")
				if err := runInstall(context.Background(), good); err != nil {
					t.Fatalf("subsequent installation blocked: %v", err)
				}
			})
		}
	}
}
