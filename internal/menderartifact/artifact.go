package menderartifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
)

type HeadersTypeInfo struct {
	ArtifactProvides map[string]any `json:"artifact_provides"`
}

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
		return "", "", fmt.Errorf("artifact_provides missing rootfs-image.version")
	}
	providesStr, ok := provides.(string)
	if !ok {
		return "", "", fmt.Errorf("unexpected type for rootfs-image.version: %T", provides)
	}
	return coreOsVersionFromRootfsImageVersion(providesStr)
}

func coreOsVersionFromRootfsImageVersion(providesStr string) (string, string, error) {
	// extract name and version from a string like "cpu01-standard-v2.6.0.f457f6d.20260210.1540"
	re := regexp.MustCompile(`^(?P<name>.+)-(?P<version>v\d+\.\d+\.\d+(?:\..+)?)$`)
	matches := re.FindStringSubmatch(providesStr)
	if matches == nil {
		return "", "", fmt.Errorf("invalid format for rootfs-image.version: %s", providesStr)
	}
	if len(matches) != 3 {
		return "", "", fmt.Errorf("unexpected regex match groups: %v", matches)
	}
	name := matches[1]
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
		return "", fmt.Errorf("artifact_provides missing %s.version", appName)
	}
	providesStr, ok := provides.(string)
	if !ok {
		return "", fmt.Errorf("unexpected type for %s.version: %T", appName, provides)
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
		return info, fmt.Errorf("artifact missing header.tar(.gz)")
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
		switch h.Name {
		case "headers/0000/type-info":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, fmt.Errorf("read headers/0000/type-info: %w", rErr)
			}
			if len(bytes.TrimSpace(b)) > 0 {
				if uErr := json.Unmarshal(b, &info); uErr != nil {
					return info, fmt.Errorf("parse headers/0000/type-info: %w", uErr)
				}
			}
			hasTypeInfo = true
		}
	}
	if !hasTypeInfo {
		return info, fmt.Errorf("type-info not found")
	}
	return info, nil
}
