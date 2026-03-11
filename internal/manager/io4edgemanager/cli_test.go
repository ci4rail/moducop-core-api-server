package io4edgemanager

import (
	"reflect"
	"testing"
)

func TestParseDevices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name: "scan output with header and devices",
			output: `DEVICE ID               IP              HARDWARE        SERIAL
S100-IUO16-USB-EXT1-UC1 192.168.200.1   s100-iou16      b4e31793-f660-4e2e-af20-c175186b95be
S100-IUO16-USB-EXT1-UC2 192.168.201.1   s100-iou16      <serial>
...`,
			want: []string{
				"S100-IUO16-USB-EXT1-UC1",
				"S100-IUO16-USB-EXT1-UC2",
			},
		},
		{
			name:   "empty output",
			output: "",
			want:   []string{},
		},
		{
			name: "ignores blank lines",
			output: `DEVICE ID               IP              HARDWARE        SERIAL

S100-IUO16-USB-EXT1-UC1 192.168.200.1   s100-iou16      b4e31793-f660-4e2e-af20-c175186b95be
`,
			want: []string{"S100-IUO16-USB-EXT1-UC1"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseDevices(tt.output)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseDevices() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseFirmwareVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   NameVersion
	}{
		{
			name:   "valid firmware output",
			output: "Firmware name: fw_iou016_default, Version 1.0.0",
			want: NameVersion{
				Name:    "fw_iou016_default",
				Version: "1.0.0",
			},
		},
		{
			name: "valid firmware output in multiline text",
			output: `some log line
Firmware name: fw_iou016_default, Version 1.0.0
another line`,
			want: NameVersion{
				Name:    "fw_iou016_default",
				Version: "1.0.0",
			},
		},
		{
			name:   "invalid firmware output",
			output: "Firmware name: fw_iou016_default Version 1.0.0",
			want:   NameVersion{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseFirmwareVersion(tt.output)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseFirmwareVersion() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
