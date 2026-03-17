package menderartifact

import (
	"path/filepath"
	"testing"
)

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
