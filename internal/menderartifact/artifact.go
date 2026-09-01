/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

// package menderartifact provides functions to read and parse Mender artifact files,
// specifically to extract version information from the artifact headers.
package menderartifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type HeadersTypeInfo struct {
	ArtifactProvides map[string]any `json:"artifact_provides"`
}

const rootfsMatchGroups = 3

var (
	errMissingRootfsImageVersion  = errors.New("artifact_provides missing rootfs-image.version")
	errUnexpectedRootfsVersionTyp = errors.New("unexpected type for rootfs-image.version")
	errInvalidRootfsVersionFormat = errors.New("invalid format for rootfs-image.version")
	errUnexpectedRootfsMatches    = errors.New("unexpected regex match groups")
	errMissingAppVersion          = errors.New("artifact_provides missing app version")
	errUnexpectedAppVersionType   = errors.New("unexpected type for app version")
	errArtifactMissingHeaderTar   = errors.New("artifact missing header.tar(.gz)")
	errTypeInfoNotFound           = errors.New("type-info not found")
	errCustomizationPayloadType   = errors.New("artifact is not an os-customization payload")
	errCustomizationManifest      = errors.New("customization payload missing manifest.json")
	errCustomizationVersion       = errors.New("customization manifest missing version")
	errArtifactMissingDataTar     = errors.New("artifact missing data/0000.tar(.gz)")
)

const headerTarGzName = "header.tar.gz"

var legacyRootfsImageVersionRe = regexp.MustCompile(`^(?P<name>.+)-image(?P<suffix>-dirty)?_(?P<version>v.+?)(?:-dev)?$`)

// CoreOSVersionFromArtifact reads the artifact file at the given path and extracts the CoreOS version
// from the artifact_provides field in the embedded header.tar(.gz) headers/0000/type-info file.
// It looks for a "provides" info for "rootfs-image.version", it then extracts the name and version
// from the value.
// Returns name, version, error
func CoreOSVersionFromArtifact(path string) (string, string, error) {
	info, err := ParseArtifactHeadersTypeInfo(path)
	if err != nil {
		return "", "", fmt.Errorf("parse artifact headers: %w", err)
	}
	provides, ok := info.ArtifactProvides["rootfs-image.version"]
	if !ok {
		return "", "", fmt.Errorf("%w", errMissingRootfsImageVersion)
	}
	providesStr, ok := provides.(string)
	if !ok {
		return "", "", fmt.Errorf("%w: %T", errUnexpectedRootfsVersionTyp, provides)
	}
	return coreOsVersionFromRootfsImageVersion(providesStr)
}

func coreOsVersionFromRootfsImageVersion(providesStr string) (string, string, error) {
	// extract name and version from a string like "cpu01-standard-v2.6.0.f457f6d.20260210.1540"
	re := regexp.MustCompile(`^(?P<name>.+)-(?P<version>v\d+\.\d+\.\d+(?:\..+)?)$`)
	matches := re.FindStringSubmatch(providesStr)
	if matches == nil {
		return "", "", fmt.Errorf("%w: %s", errInvalidRootfsVersionFormat, providesStr)
	}
	if len(matches) != rootfsMatchGroups {
		return "", "", fmt.Errorf("%w: %v", errUnexpectedRootfsMatches, matches)
	}
	name := matches[1]
	name = strings.TrimSuffix(name, "-dirty")

	version := matches[2]
	return name, version, nil
}

// AppVersionFromArtifact reads the artifact file at the given path and extracts the application version
// for the given appName from the artifact_provides field in the embedded header.tar(.gz) headers/0000/type-info file.
// It looks for a "provides" info for "data-partition.<appName>.version", and returns its value as a string.
func AppVersionFromArtifact(path string, appName string) (string, error) {
	info, err := ParseArtifactHeadersTypeInfo(path)
	if err != nil {
		return "", fmt.Errorf("parse artifact headers: %w", err)
	}
	provides, ok := info.ArtifactProvides[fmt.Sprintf("data-partition.%s.version", appName)]
	if !ok {
		return "", fmt.Errorf("%w: %s.version", errMissingAppVersion, appName)
	}
	providesStr, ok := provides.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s.version: %T", errUnexpectedAppVersionType, appName, provides)
	}
	return providesStr, nil
}

// CoreOSCustomizationVersionFromArtifact validates an os-customization artifact
// and returns the version in its manifest-only payload.
//
//nolint:cyclop // Mender artifacts may carry compressed or uncompressed headers and payloads.
func CoreOSCustomizationVersionFromArtifact(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	tr := tar.NewReader(f)
	var headerTar, dataTar []byte
	var dataTarGz bool
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read artifact tar: %w", err)
		}
		switch h.Name {
		case "header.tar", headerTarGzName:
			headerTar, err = io.ReadAll(tr)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", h.Name, err)
			}
			if h.Name == headerTarGzName {
				gr, err := gzip.NewReader(bytes.NewReader(headerTar))
				if err != nil {
					return "", fmt.Errorf("open header.tar.gz: %w", err)
				}
				headerTar, err = io.ReadAll(gr)
				_ = gr.Close()
				if err != nil {
					return "", fmt.Errorf("read header.tar.gz: %w", err)
				}
			}
		case "data/0000.tar", "data/0000.tar.gz":
			dataTar, err = io.ReadAll(tr)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", h.Name, err)
			}
			dataTarGz = h.Name == "data/0000.tar.gz"
		}
	}
	if !customizationPayloadType(headerTar) {
		return "", errCustomizationPayloadType
	}
	if len(dataTar) == 0 {
		return "", errArtifactMissingDataTar
	}
	if dataTarGz {
		gr, err := gzip.NewReader(bytes.NewReader(dataTar))
		if err != nil {
			return "", fmt.Errorf("open data/0000.tar.gz: %w", err)
		}
		dataTar, err = io.ReadAll(gr)
		_ = gr.Close()
		if err != nil {
			return "", fmt.Errorf("read data/0000.tar.gz: %w", err)
		}
	}
	return customizationVersionFromTar(dataTar)
}

func customizationPayloadType(headerTar []byte) bool {
	tr := tar.NewReader(bytes.NewReader(headerTar))
	for {
		h, err := tr.Next()
		if err != nil {
			return false
		}
		if h.Name != "header-info" {
			continue
		}
		var info struct {
			Payloads []struct {
				Type string `json:"type"`
			} `json:"payloads"`
		}
		if err := json.NewDecoder(tr).Decode(&info); err != nil {
			return false
		}
		return len(info.Payloads) == 1 && info.Payloads[0].Type == "os-customization"
	}
}

//nolint:cyclop,nestif // Supports the direct and nested payload archive forms produced by Mender modules.
func customizationVersionFromTar(data []byte) (string, error) {
	tr := tar.NewReader(bytes.NewReader(data))
	var nested []byte
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read customization payload: %w", err)
		}
		name := strings.TrimPrefix(strings.TrimPrefix(h.Name, "./"), "/")
		if name == "manifest.json" {
			var manifest struct {
				Version string `json:"version"`
			}
			if err := json.NewDecoder(tr).Decode(&manifest); err != nil {
				return "", fmt.Errorf("parse customization manifest: %w", err)
			}
			if manifest.Version == "" {
				return "", errCustomizationVersion
			}
			return manifest.Version, nil
		}
		if h.Typeflag == tar.TypeReg && nested == nil && (strings.HasSuffix(name, ".tar") || strings.HasSuffix(name, ".tar.gz")) {
			nested, err = io.ReadAll(tr)
			if err != nil {
				return "", fmt.Errorf("read nested customization payload: %w", err)
			}
			if strings.HasSuffix(name, ".tar.gz") {
				gr, err := gzip.NewReader(bytes.NewReader(nested))
				if err != nil {
					return "", fmt.Errorf("open nested customization payload: %w", err)
				}
				nested, err = io.ReadAll(gr)
				_ = gr.Close()
				if err != nil {
					return "", fmt.Errorf("read nested customization payload: %w", err)
				}
			}
		}
	}
	if nested != nil {
		return customizationVersionFromTar(nested)
	}
	return "", errCustomizationManifest
}

// ParseArtifactHeadersTypeInfo reads the artifact file at the given
// path and extracts the artifact_provides information from the
// embedded header.tar(.gz) headers/0000/type-info file.
// Within the type-info file, it looks for the artifact_provides field and returns it as a map.
func ParseArtifactHeadersTypeInfo(path string) (HeadersTypeInfo, error) {
	var info HeadersTypeInfo

	f, err := os.Open(path)
	if err != nil {
		return info, err
	}
	defer f.Close()

	tr := tar.NewReader(f)
	var headerTar []byte

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return info, fmt.Errorf("read artifact tar: %w", err)
		}

		switch h.Name {
		case "header.tar.gz":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, fmt.Errorf("read header.tar.gz: %w", rErr)
			}
			return parseHeaderTarGz(b)
		case "header.tar":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, fmt.Errorf("read header.tar: %w", rErr)
			}
			headerTar = b
		}
	}

	if len(headerTar) == 0 {
		return info, fmt.Errorf("%w", errArtifactMissingHeaderTar)
	}
	return parseHeaderTar(bytes.NewReader(headerTar))
}

func parseHeaderTarGz(data []byte) (HeadersTypeInfo, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return HeadersTypeInfo{}, fmt.Errorf("open header.tar.gz: %w", err)
	}
	defer gr.Close()
	return parseHeaderTar(gr)
}

func parseHeaderTar(r io.Reader) (HeadersTypeInfo, error) {
	var info HeadersTypeInfo
	var hasTypeInfo bool

	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return info, fmt.Errorf("read header tar: %w", err)
		}
		if h.Name != "headers/0000/type-info" {
			continue
		}
		b, rErr := io.ReadAll(tr)
		if rErr != nil {
			return info, fmt.Errorf("read headers/0000/type-info: %w", rErr)
		}
		if len(bytes.TrimSpace(b)) > 0 {
			if uErr := json.Unmarshal(b, &info); uErr != nil {
				return info, fmt.Errorf("parse headers/0000/type-info: %w", uErr)
			}
			normalizeArtifactProvides(info.ArtifactProvides)
		}
		hasTypeInfo = true
	}
	if !hasTypeInfo {
		return info, fmt.Errorf("%w", errTypeInfoNotFound)
	}
	return info, nil
}

func normalizeArtifactProvides(provides map[string]any) {
	if provides == nil {
		return
	}

	rawVersion, ok := provides["rootfs-image.version"]
	if !ok {
		return
	}

	version, ok := rawVersion.(string)
	if !ok {
		return
	}

	matches := legacyRootfsImageVersionRe.FindStringSubmatch(version)
	if matches == nil {
		return
	}

	provides["rootfs-image.version"] = matches[1] + matches[2] + "-" + matches[3]
}
