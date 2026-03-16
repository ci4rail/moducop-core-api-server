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
)

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
