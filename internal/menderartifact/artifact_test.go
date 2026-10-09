/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package menderartifact

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCoreOSCustomizationVersionFromArtifact(t *testing.T) {
	artifact := customizationArtifact(t, "site-1.2.3", "os-customization")
	version, err := CoreOSCustomizationVersionFromArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if version != "site-1.2.3" {
		t.Fatalf("version = %q, want site-1.2.3", version)
	}
}

func TestCoreOSCustomizationVersionFromArtifactRejectsWrongPayload(t *testing.T) {
	artifact := customizationArtifact(t, "site-1.2.3", "app")
	if _, err := CoreOSCustomizationVersionFromArtifact(artifact); err == nil {
		t.Fatal("CoreOSCustomizationVersionFromArtifact accepted an app artifact")
	}
}

func customizationArtifact(t *testing.T, version, payloadType string) string {
	t.Helper()
	header := tarForTest(t, map[string]string{
		"header-info": `{"payloads":[{"type":"` + payloadType + `"}]}`,
	})
	payload := tarForTest(t, map[string]string{
		"manifest.json": `{"format_version":1,"version":"` + version + `"}`,
	})
	artifact := tarForTest(t, map[string]string{
		"header.tar":    string(header),
		"data/0000.tar": string(payload),
	})
	path := filepath.Join(t.TempDir(), "customization.mender")
	if err := os.WriteFile(path, artifact, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func tarForTest(t *testing.T, files map[string]string) []byte {
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

func TestParseArtifactHeadersTypeInfo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		artifactPath  string
		providesField string
		providesValue string
	}{
		{
			name: "CPU01 Rootfs Artifact",
			artifactPath: filepath.Join(
				"..", "..", "tests", "assets",
				"Moducop-CPU01_Standard-Image_v2.6.0.f457f6d.20260210.1540.mender",
			),
			providesField: "rootfs-image.version",
			providesValue: "cpu01-standard-v2.6.0.f457f6d.20260210.1540",
		},
		{
			name: "CPU01 Rootfs Artifact2",
			artifactPath: filepath.Join(
				"..", "..", "tests", "assets",
				"Moducop-CPU01_Standard-Image_dirty_v2.7.0.some_dummy_change.40ee657.klaus.20260313.1713.mender",
			),
			providesField: "rootfs-image.version",
			providesValue: "cpu01-standard-dirty-v2.7.0.some_dummy_change.40ee657.klaus.20260313.1713",
		},
		{
			name: "Nginx Demo App Artifact",
			artifactPath: filepath.Join(
				"..", "..", "tests", "assets",
				"app-nginx-demo-moducop-cpu01-linux_arm64-a895c3c.mender",
			),
			providesField: "data-partition.nginx-demo.version",
			providesValue: "a895c3c",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			info, err := ParseArtifactHeadersTypeInfo(tc.artifactPath)
			if err != nil {
				t.Fatalf("ParseArtifactHeadersTypeInfo() error = %v", err)
			}
			if value, ok := info.ArtifactProvides[tc.providesField]; !ok {
				t.Fatalf("missing artifact_provides field: %s", tc.providesField)
			} else if value != tc.providesValue {
				t.Fatalf("unexpected artifact_provides: got %v, want %v", value, tc.providesValue)
			}

			// t.Logf("artifact=%s err=%v artifact_provides=%q", tc.artifactPath, err, info.ArtifactProvides)
		})
	}
}

func TestCoreOsVersionFromRootfsImageVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		providesStr     string
		expectedName    string
		expectedVersion string
		expectError     bool
	}{
		{
			name:            "valid format",
			providesStr:     "cpu01-standard-v2.6.0.f457f6d.20260210.1540",
			expectedName:    "cpu01-standard",
			expectedVersion: "v2.6.0.f457f6d.20260210.1540",
			expectError:     false,
		},
		{
			name:            "valid format2",
			providesStr:     "cpu01-standard-mod-v2.6.0",
			expectedName:    "cpu01-standard-mod",
			expectedVersion: "v2.6.0",
			expectError:     false,
		},
		{
			name:        "invalid format - missing version",
			providesStr: "cpu01-standard",
			expectError: true,
		},
		{
			name:        "invalid format - missing name",
			providesStr: "-v2.6.0",
			expectError: true,
		},
		{
			name:        "invalid format - no hyphen",
			providesStr: "cpu01standardv2.6.0",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			name, version, err := coreOsVersionFromRootfsImageVersion(tc.providesStr)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tc.expectedName {
				t.Errorf("unexpected name: got %s, want %s", name, tc.expectedName)
			}
			if version != tc.expectedVersion {
				t.Errorf("unexpected version: got %s, want %s", version, tc.expectedVersion)
			}
		})
	}
}

func TestCoreOSVersionFromArtifact(t *testing.T) {
	t.Parallel()

	artifactPath := filepath.Join(
		"..", "..", "tests", "assets",
		"Moducop-CPU01_Standard-Image_v2.6.0.f457f6d.20260210.1540.mender",
	)

	name, version, err := CoreOSVersionFromArtifact(artifactPath)
	if err != nil {
		t.Fatalf("CoreOSVersionFromArtifact() error = %v", err)
	}
	expectedName := "cpu01-standard"
	expectedVersion := "v2.6.0.f457f6d.20260210.1540"
	if name != expectedName {
		t.Errorf("unexpected name: got %s, want %s", name, expectedName)
	}
	if version != expectedVersion {
		t.Errorf("unexpected version: got %s, want %s", version, expectedVersion)
	}
}

func TestAppVersionFromArtifact(t *testing.T) {
	t.Parallel()

	artifactPath := filepath.Join(
		"..", "..", "tests", "assets",
		"app-nginx-demo-moducop-cpu01-linux_arm64-a895c3c.mender",
	)

	appName := "nginx-demo"
	version, err := AppVersionFromArtifact(artifactPath, appName)
	if err != nil {
		t.Fatalf("AppVersionFromArtifact() error = %v", err)
	}
	expectedVersion := "a895c3c"
	if version != expectedVersion {
		t.Errorf("unexpected version: got %s, want %s", version, expectedVersion)
	}
}

func TestCoreOSVersionMetadataFormats(t *testing.T) {
	cases := []struct{ name, key, raw, version string }{
		{"legacy canonical", "rootfs-image.version", "cpu01-standard-v2.6.0.f457f6d.20260210.1540", "v2.6.0.f457f6d.20260210.1540"},
		{"legacy image label", "rootfs-image.version", "cpu01-standard-image-dirty_v2.7.0.some_dummy_change-dev", "v2.7.0.some_dummy_change"},
		{"bootfit dirty", "rootfs-image.bootfit-rootfs.version", "cpu01-standard-image-dirty_v3.0.0-alpha.1+2.scarthgap-secure-boot-mender-signing.4407d1c.klaus.20261006.1241-dev", "v3.0.0-alpha.1+2.scarthgap-secure-boot-mender-signing.4407d1c.klaus.20261006.1241"},
		{"bootfit clean", "rootfs-image.bootfit-rootfs.version", "cpu01-standard-image-v3.0.0-alpha.1+4.scarthgap-secure-boot-mender-signing.7b4da3c.20261006.1424-dev", "v3.0.0-alpha.1+4.scarthgap-secure-boot-mender-signing.7b4da3c.20261006.1424"},
		{"canonical prerelease", "rootfs-image.version", "cpu01-standard-v3.0.0-alpha.1+build.42", "v3.0.0-alpha.1+build.42"},
		{"canonical dev prerelease retained", "rootfs-image.version", "cpu01-standard-v3.0.0-dev", "v3.0.0-dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metadata, err := json.Marshal(HeadersTypeInfo{ArtifactProvides: map[string]any{tc.key: tc.raw}})
			if err != nil {
				t.Fatal(err)
			}
			header := tarForTest(t, map[string]string{"headers/0000/type-info": string(metadata)})
			data := tarForTest(t, map[string]string{"header.tar": string(header)})
			path := filepath.Join(t.TempDir(), "rootfs.mender")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			name, version, err := CoreOSVersionFromArtifact(path)
			if err != nil || name != "cpu01-standard" || version != tc.version {
				t.Fatalf("got %q %q %v; want cpu01-standard %q", name, version, err, tc.version)
			}
		})
	}
}

func TestCoreOSVersionMetadataRejectsInvalidValues(t *testing.T) {
	for _, provides := range []map[string]any{
		{"rootfs-image.bootfit-rootfs.version": 42},
		{"rootfs-image.bootfit-rootfs.version": "cpu01-standard-image-not-a-version"},
		{"rootfs-image.bootfit-rootfs.version": "cpu01-standard-v3.0.0-"},
		{"rootfs-image.bootfit-rootfs.version": "cpu01-standard-v3.0.0-alpha with spaces"},
		{"unrelated.version": "v3.0.0"},
	} {
		metadata, err := json.Marshal(HeadersTypeInfo{ArtifactProvides: provides})
		if err != nil {
			t.Fatal(err)
		}
		header := tarForTest(t, map[string]string{"headers/0000/type-info": string(metadata)})
		path := filepath.Join(t.TempDir(), "invalid.mender")
		if err := os.WriteFile(path, tarForTest(t, map[string]string{"header.tar": string(header)}), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := CoreOSVersionFromArtifact(path); err == nil {
			t.Fatalf("accepted invalid metadata: %+v", provides)
		}
	}
}
