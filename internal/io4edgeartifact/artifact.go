/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

// package io4edgeartifact provides functions to read io4edge fw packages,
// specifically to extract version information from the manifest.json.
package io4edgeartifact

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type FirmwareManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	File    string `json:"file"`
}

var (
	errMissingManifestJSON   = errors.New("invalid firmware package: missing manifest.json")
	errMissingFirmwareBinary = errors.New("invalid firmware package: missing firmware binary")
)

func GetManifestFromFile(firmwarePath string) (FirmwareManifest, error) {
	f, err := os.Open(firmwarePath)
	if err != nil {
		return FirmwareManifest{}, err
	}
	defer func() {
		_ = f.Close()
	}()

	manifestRaw, entryNames, err := extractManifestAndEntries(tar.NewReader(f))
	if err != nil {
		return FirmwareManifest{}, err
	}

	var manifest FirmwareManifest
	if len(manifestRaw) == 0 {
		return FirmwareManifest{}, fmt.Errorf("%w", errMissingManifestJSON)
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return FirmwareManifest{}, fmt.Errorf("invalid firmware manifest: %w", err)
	}
	if manifest.File != "" && !entryNames[manifest.File] {
		return FirmwareManifest{}, fmt.Errorf("%w", errMissingFirmwareBinary)
	}
	return manifest, nil
}

func extractManifestAndEntries(tr *tar.Reader) ([]byte, map[string]bool, error) {
	manifestRaw := []byte{}
	entryNames := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return manifestRaw, entryNames, nil
		}
		if err != nil {
			return nil, nil, err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		entryNames[name] = true
		if name != "manifest.json" {
			continue
		}
		manifestRaw, err = io.ReadAll(tr)
		if err != nil {
			return nil, nil, err
		}
	}
}
