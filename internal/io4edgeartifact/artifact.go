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

func GetManifestFromFile(firmwarePath string) (FirmwareManifest, error) {
	f, err := os.Open(firmwarePath)
	if err != nil {
		return FirmwareManifest{}, err
	}
	defer func() {
		_ = f.Close()
	}()

	tr := tar.NewReader(f)
	var (
		manifestRaw []byte
		entryNames  = map[string]bool{}
		manifest    FirmwareManifest
	)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return FirmwareManifest{}, err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		entryNames[name] = true
		switch name {
		case "manifest.json":
			manifestRaw, err = io.ReadAll(tr)
			if err != nil {
				return FirmwareManifest{}, err
			}
		}
	}
	if len(manifestRaw) == 0 {
		return FirmwareManifest{}, errors.New("invalid firmware package: missing manifest.json")
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return FirmwareManifest{}, fmt.Errorf("invalid firmware manifest: %w", err)
	}
	if manifest.File != "" && !entryNames[manifest.File] {
		return FirmwareManifest{}, errors.New("invalid firmware package: missing firmware binary")
	}
	return manifest, nil
}
