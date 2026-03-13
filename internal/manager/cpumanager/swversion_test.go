package cpumanager

import (
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
