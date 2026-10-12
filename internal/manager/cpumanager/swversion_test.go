/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cpumanager

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCoreOsVersionFromIssueLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantName    string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "valid line with underscores",
			line:        "Moducop-CPU01_Standard-Image_v2.6.0.f457f6d.20260210.1540",
			wantName:    "cpu01-standard",
			wantVersion: "v2.6.0.f457f6d.20260210.1540",
			wantErr:     false,
		},
		{
			name:        "valid line without underscores2",
			line:        "Moducop-CPU01-Standard-Image_v2.6.0.f457f6d.20260210.1540",
			wantName:    "cpu01-standard",
			wantVersion: "v2.6.0.f457f6d.20260210.1540",
			wantErr:     false,
		},
		{
			name:        "valid line without underscores3",
			line:        "Moducop-CPU01_Standard-Image_dirty_v2.7.0.some_dummy_change.40ee657.klaus.20260313.1713",
			wantName:    "cpu01-standard",
			wantVersion: "v2.7.0.some_dummy_change.40ee657.klaus.20260313.1713",
			wantErr:     false,
		},
		{
			name:        "invalid line format",
			line:        "Invalid issue line",
			wantName:    "",
			wantVersion: "",
			wantErr:     true,
		},
		{
			name:        "missing version",
			line:        "Moducop-CPU01_Standard-Image_",
			wantName:    "",
			wantVersion: "",
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVersion, err := coreOsVersionFromIssueLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("coreOsVersionFromIssueLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotName != tt.wantName {
				t.Errorf("coreOsVersionFromIssueLine() gotName = %v, want %v", gotName, tt.wantName)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("coreOsVersionFromIssueLine() gotVersion = %v, want %v", gotVersion, tt.wantVersion)
			}
		})
	}
}

func TestAppVersionFromData(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "valid data",
			data:        "MY_ENV_VAR=foo\nSOFTWARE_VERSION=1.2.3\nOTHER_VAR=bar",
			wantVersion: "1.2.3",
			wantErr:     false,
		},
		{
			name:        "invalid data format",
			data:        "INVALID_DATA",
			wantVersion: "",
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVersion, err := appVersionFromData(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("appVersionFromData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("appVersionFromData() gotVersion = %v, want %v", gotVersion, tt.wantVersion)
			}
		})
	}
}

func TestNonSecureBootIssueVersions(t *testing.T) {
	for _, tc := range []struct{ line, version string }{
		{"Moducop-CPU01_Standard-Image-Non-Secure-Boot_dirty_v3.0.0-alpha.1+2.scarthgap-secure-boot-mender-signing.4407d1c.klaus.20261006.1241", "v3.0.0-alpha.1+2.scarthgap-secure-boot-mender-signing.4407d1c.klaus.20261006.1241"},
		{"Moducop-CPU01_Standard-Image-Non-Secure-Boot_v3.0.0-alpha.1+4.scarthgap-secure-boot-mender-signing.7b4da3c.20261006.1424", "v3.0.0-alpha.1+4.scarthgap-secure-boot-mender-signing.7b4da3c.20261006.1424"},
		{"Moducop-CPU01_Standard-Image_v3.0.0-alpha.1+build.42", "v3.0.0-alpha.1+build.42"},
	} {
		name, version, err := coreOsVersionFromIssueLine(tc.line)
		if err != nil || name != "cpu01-standard" || version != tc.version {
			t.Fatalf("got %q %q %v; want cpu01-standard %q", name, version, err, tc.version)
		}
	}
	for _, line := range []string{"Moducop-CPU01_Standard-Image-Non-Secure-Boot_v3.0.0-", "Moducop-CPU01_Standard-Image-Non-Secure-Boot_v3.0.0-alpha with spaces"} {
		if _, _, err := coreOsVersionFromIssueLine(line); err == nil {
			t.Fatalf("accepted %q", line)
		}
	}
}

func TestListApplicationsExcludesTransactions(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MOCK_MENDER_STATE_DIR", root)
	for _, name := range []string{"demo", "other", ".transactions", "demo-previous", "demo-last"} {
		if err := os.MkdirAll(filepath.Join(root, "fs/data/mender-app", name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	apps, err := listApplicationsFromTargetFS()
	if err != nil || !reflect.DeepEqual(apps, []string{"demo", "other"}) {
		t.Fatalf("apps %v, error %v", apps, err)
	}
}
