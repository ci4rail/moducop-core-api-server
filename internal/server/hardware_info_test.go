/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import "testing"

func TestParseHardwareInfo_ValidJSON(t *testing.T) {
	in := `{
  "vendor": "Ci4Rail",
  "model": "S100-MLC01",
  "variant": 0,
  "majorVersion": 1,
  "serial": "12345"
}`

	got, err := parseHardwareInfo(in)
	if err != nil {
		t.Fatalf("expected no error, got=%v", err)
	}
	want := hardwareInfo{
		Vendor:       "Ci4Rail",
		Model:        "S100-MLC01",
		Variant:      0,
		MajorVersion: 1,
		Serial:       "12345",
	}

	if got != want {
		t.Fatalf("unexpected parse result: got=%+v want=%+v", got, want)
	}
}

func TestParseHardwareInfo_InvalidJSON(t *testing.T) {
	got, err := parseHardwareInfo("not-json")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if got != (hardwareInfo{}) {
		t.Fatalf("expected zero value for invalid json, got=%+v", got)
	}
}
