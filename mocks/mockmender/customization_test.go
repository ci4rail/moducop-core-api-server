/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package mockmender

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParseCustomizationArtifactHealthChecks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		manifest string
		passed   bool
	}{
		{
			name:     "true command array passes",
			manifest: `{"version":"good-1","health_checks":[{"type":"command","command":["true"]}]}`,
			passed:   true,
		},
		{
			name:     "false command string fails",
			manifest: `{"version":"bad-1","health_checks":[{"type":"command","command":"false"}]}`,
			passed:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			artifact := writeCustomizationArtifact(t, tc.manifest)
			info, manifest, passed, err := ParseCustomizationArtifact(artifact)
			if err != nil {
				t.Fatal(err)
			}
			if info.Payloads[0].Type != string(UpdateTypeCustomization) {
				t.Fatalf("payload type = %q", info.Payloads[0].Type)
			}
			if manifest.Version == "" || passed != tc.passed {
				t.Fatalf("manifest version = %q, passed = %v", manifest.Version, passed)
			}
		})
	}
}

func TestParseCustomizationArtifactRejectsUnsupportedCommand(t *testing.T) {
	artifact := writeCustomizationArtifact(t, `{"version":"bad-command","health_checks":[{"type":"command","command":["echo","ok"]}]}`)
	if _, _, _, err := ParseCustomizationArtifact(artifact); err == nil {
		t.Fatal("ParseCustomizationArtifact succeeded for an unsupported command")
	}
}

func TestParseCustomizationArtifactReadsNestedPayloadArchive(t *testing.T) {
	payload := tarBytes(t, map[string]string{"manifest.json": `{"version":"nested-1","health_checks":[{"type":"command","command":["true"]}]}`})
	header := tarBytes(t, map[string]string{
		"header-info": `{"payloads":[{"type":"os-customization"}],"artifact_depends":{"device_type":["moducop-cpu01"]}}`,
	})
	artifact := tarBytes(t, map[string]string{
		"header.tar":    string(header),
		"data/0000.tar": string(tarBytes(t, map[string]string{"customization.tar": string(payload)})),
	})
	path := filepath.Join(t.TempDir(), "customization.mender")
	if err := os.WriteFile(path, artifact, 0o600); err != nil {
		t.Fatal(err)
	}
	_, manifest, passed, err := ParseCustomizationArtifact(path)
	if err != nil || manifest.Version != "nested-1" || !passed {
		t.Fatalf("manifest = %#v, passed = %v, err = %v", manifest, passed, err)
	}
}

func TestCustomizationLifecycleUpdatesStatusState(t *testing.T) {
	s := State{}
	SetInstalledCustomization(&s, "good.mender", "good-1", true)
	if s.CandidateCustomizationSlot != "A" || s.CustomizationCandidateState != "pending" {
		t.Fatalf("staged state = %#v", s)
	}

	EvaluateCustomizationHealth(&s, true)
	if s.ActiveCustomizationVersion != "good-1" || s.ActiveCustomizationSlot != "A" || s.CustomizationCandidateState != "" {
		t.Fatalf("successful reboot state = %#v", s)
	}
	CommitCustomization(&s)
	if s.Stage != stageIdle || s.PendingUpdateType != string(UpdateTypeNone) || s.ActiveCustomizationVersion != "good-1" {
		t.Fatalf("committed state = %#v", s)
	}

	SetInstalledCustomization(&s, "bad.mender", "bad-1", false)
	EvaluateCustomizationHealth(&s, false)
	if s.ActiveCustomizationVersion != "good-1" || s.CustomizationCandidateState != "rolled-back" {
		t.Fatalf("failed reboot state = %#v", s)
	}
	RollbackCustomization(&s)
	if s.Stage != stageIdle || s.CustomizationCandidateState != "rolled-back" {
		t.Fatalf("rolled back state = %#v", s)
	}
}

func writeCustomizationArtifact(t *testing.T, manifest string) string {
	t.Helper()
	payload := tarBytes(t, map[string]string{"manifest.json": manifest})
	header := tarBytes(t, map[string]string{
		"header-info": `{"payloads":[{"type":"os-customization"}],"artifact_depends":{"device_type":["moducop-cpu01"]}}`,
	})
	artifact := tarBytes(t, map[string]string{
		"header.tar":    string(header),
		"data/0000.tar": string(payload),
	})
	path := filepath.Join(t.TempDir(), "customization.mender")
	if err := os.WriteFile(path, artifact, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func tarBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
