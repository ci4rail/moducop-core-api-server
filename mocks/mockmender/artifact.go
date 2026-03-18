/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package mockmender

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	maxRootfsExtractedTarFileSize   = 2 * 1024 * 1024 * 1024
	maxManifestExtractedTarFileSize = 256 * 1024 * 1024
)

type HeaderInfo struct {
	Payloads []struct {
		Type string `json:"type"`
	} `json:"payloads"`
	ArtifactProvides struct {
		ArtifactName string `json:"artifact_name"`
	} `json:"artifact_provides"`
	ArtifactDepends struct {
		DeviceType []string `json:"device_type"`
	} `json:"artifact_depends"`
}

type AppMetaData struct {
	ApplicationName string   `json:"application_name"`
	ProjectName     string   `json:"project_name"`
	Images          []string `json:"images"`
	Orchestrator    string   `json:"orchestrator"`
	Platform        string   `json:"platform"`
	Version         string   `json:"version"`
}

func ParseArtifactHeader(path string) (HeaderInfo, AppMetaData, error) {
	var info HeaderInfo
	var metadata AppMetaData

	f, err := os.Open(path)
	if err != nil {
		return info, metadata, err
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
			return info, metadata, fmt.Errorf("read artifact tar: %w", err)
		}

		switch h.Name {
		case "header.tar.gz":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, metadata, fmt.Errorf("read header.tar.gz: %w", rErr)
			}
			return parseHeaderTarGz(b)
		case "header.tar":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, metadata, fmt.Errorf("read header.tar: %w", rErr)
			}
			headerTar = b
		}
	}

	if len(headerTar) == 0 {
		return info, metadata, fmt.Errorf("artifact missing header.tar(.gz)")
	}
	return parseHeaderTar(bytes.NewReader(headerTar))
}

func ParseAndExtractArtifact(path string) (HeaderInfo, string, error) {
	var info HeaderInfo

	f, err := os.Open(path)
	if err != nil {
		return info, "", err
	}
	defer f.Close()

	tr := tar.NewReader(f)
	var headerTar []byte
	var rootfsData []byte

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return info, "", fmt.Errorf("read artifact tar: %w", err)
		}

		switch h.Name {
		case "header.tar.gz":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, "", fmt.Errorf("read header.tar.gz: %w", rErr)
			}
			parsed, _, pErr := parseHeaderTarGz(b)
			if pErr != nil {
				return info, "", pErr
			}
			info = parsed
		case "header.tar":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, "", fmt.Errorf("read header.tar: %w", rErr)
			}
			headerTar = b
		case "data/0000.tar.gz":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, "", fmt.Errorf("read data/0000.tar.gz: %w", rErr)
			}
			rootfsData = b
		}
	}

	if len(info.Payloads) == 0 && len(headerTar) > 0 {
		parsed, _, pErr := parseHeaderTar(bytes.NewReader(headerTar))
		if pErr != nil {
			return info, "", pErr
		}
		info = parsed
	}
	if len(info.Payloads) == 0 {
		return info, "", fmt.Errorf("artifact missing header-info")
	}
	if len(rootfsData) == 0 {
		return info, "", fmt.Errorf("artifact missing data/0000.tar.gz")
	}

	outDir, err := os.MkdirTemp("", "mock-mender-rootfs-*")
	if err != nil {
		return info, "", err
	}
	if err := extractTarGzBytes(rootfsData, outDir); err != nil {
		return info, "", err
	}
	return info, outDir, nil
}

func ParseAndExtractAppArtifact(path string, appManifestDir string) (HeaderInfo, AppMetaData, error) {
	var info HeaderInfo
	var metadata AppMetaData

	f, err := os.Open(path)
	if err != nil {
		return info, metadata, err
	}
	defer f.Close()

	tr := tar.NewReader(f)
	var headerTar []byte
	var dataTar []byte
	var dataTarGz []byte

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return info, metadata, fmt.Errorf("read artifact tar: %w", err)
		}

		switch h.Name {
		case "header.tar.gz":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, metadata, fmt.Errorf("read header.tar.gz: %w", rErr)
			}
			parsed, meta, pErr := parseHeaderTarGz(b)
			if pErr != nil {
				return info, metadata, pErr
			}
			info = parsed
			metadata = meta
		case "header.tar":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, metadata, fmt.Errorf("read header.tar: %w", rErr)
			}
			headerTar = b
		case "data/0000.tar.gz":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, metadata, fmt.Errorf("read data/0000.tar.gz: %w", rErr)
			}
			dataTarGz = b
		case "data/0000.tar":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, metadata, fmt.Errorf("read data/0000.tar: %w", rErr)
			}
			dataTar = b
		}
	}

	if len(info.Payloads) == 0 && len(headerTar) > 0 {
		parsed, meta, pErr := parseHeaderTar(bytes.NewReader(headerTar))
		if pErr != nil {
			return info, metadata, pErr
		}
		info = parsed
		metadata = meta
	}
	if len(info.Payloads) == 0 {
		return info, metadata, fmt.Errorf("artifact missing header-info")
	}
	if len(dataTar) == 0 && len(dataTarGz) == 0 {
		return info, metadata, fmt.Errorf("artifact missing data/0000.tar or data/0000.tar.gz")
	}
	if metadata.ApplicationName == "" && metadata.ProjectName == "" {
		return info, metadata, fmt.Errorf("artifact missing application_name in headers/0000/meta-data")
	}

	if err := extractManifestTarFromData(dataTar, dataTarGz, appManifestDir); err != nil {
		return info, metadata, err
	}
	return info, metadata, nil
}

func parseHeaderTarGz(data []byte) (HeaderInfo, AppMetaData, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return HeaderInfo{}, AppMetaData{}, fmt.Errorf("open header.tar.gz: %w", err)
	}
	defer gr.Close()
	return parseHeaderTar(gr)
}

func parseHeaderTar(r io.Reader) (HeaderInfo, AppMetaData, error) {
	var info HeaderInfo
	var metadata AppMetaData
	var hasHeaderInfo bool

	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return info, metadata, fmt.Errorf("read header tar: %w", err)
		}
		switch h.Name {
		case "header-info":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, metadata, fmt.Errorf("read header-info: %w", rErr)
			}
			if uErr := json.Unmarshal(b, &info); uErr != nil {
				return info, metadata, fmt.Errorf("parse header-info: %w", uErr)
			}
			hasHeaderInfo = true
		case "headers/0000/meta-data":
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, metadata, fmt.Errorf("read headers/0000/meta-data: %w", rErr)
			}
			if len(bytes.TrimSpace(b)) > 0 {
				if uErr := json.Unmarshal(b, &metadata); uErr != nil {
					return info, metadata, fmt.Errorf("parse headers/0000/meta-data: %w", uErr)
				}
			}
		}
	}
	if !hasHeaderInfo {
		return info, metadata, fmt.Errorf("header-info not found")
	}
	return info, metadata, nil
}

func extractTarGzBytes(data []byte, outDir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(outDir, filepath.Clean(h.Name))
		rel, err := filepath.Rel(outDir, target)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("unsafe path in payload: %s", h.Name)
		}

		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if err := copyTarEntryWithLimit(f, tr, h.Name, h.Size, maxRootfsExtractedTarFileSize); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractManifestTarFromData(dataTar []byte, dataTarGz []byte, appManifestDir string) error {
	if err := os.MkdirAll(appManifestDir, 0o755); err != nil {
		return err
	}

	var tr *tar.Reader
	if len(dataTar) > 0 {
		tr = tar.NewReader(bytes.NewReader(dataTar))
	} else {
		gr, err := gzip.NewReader(bytes.NewReader(dataTarGz))
		if err != nil {
			return fmt.Errorf("open data/0000.tar.gz: %w", err)
		}
		defer gr.Close()
		tr = tar.NewReader(gr)
	}

	var manifestsTar []byte
	var manifestsTarGz []byte
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if len(dataTar) > 0 {
				return fmt.Errorf("read data/0000.tar: %w", err)
			}
			return fmt.Errorf("read data/0000.tar.gz: %w", err)
		}
		switch h.Name {
		case "manifests.tar":
			manifestsTar, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("read manifests.tar: %w", err)
			}
		case "manifests.tar.gz":
			manifestsTarGz, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("read manifests.tar.gz: %w", err)
			}
		}
		if len(manifestsTar) > 0 || len(manifestsTarGz) > 0 {
			break
		}
	}

	if len(manifestsTar) == 0 && len(manifestsTarGz) == 0 {
		return fmt.Errorf("data/0000.tar missing manifests.tar(.gz)")
	}
	if len(manifestsTar) == 0 {
		gr, err := gzip.NewReader(bytes.NewReader(manifestsTarGz))
		if err != nil {
			return fmt.Errorf("open manifests.tar.gz: %w", err)
		}
		defer gr.Close()
		manifestsTar, err = io.ReadAll(gr)
		if err != nil {
			return fmt.Errorf("read manifests.tar.gz payload: %w", err)
		}
	}
	return extractManifestTar(manifestsTar, appManifestDir)
}

func extractManifestTar(manifestsTar []byte, appManifestDir string) error {
	tr := tar.NewReader(bytes.NewReader(manifestsTar))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read manifests.tar: %w", err)
		}

		name := strings.TrimPrefix(filepath.Clean(h.Name), "manifests/")
		if name == "." || name == "" {
			continue
		}
		target := filepath.Join(appManifestDir, name)
		rel, err := filepath.Rel(appManifestDir, target)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("unsafe path in manifests.tar: %s", h.Name)
		}

		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if err := copyTarEntryWithLimit(f, tr, h.Name, h.Size, maxManifestExtractedTarFileSize); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
}

func copyTarEntryWithLimit(dst io.Writer, src io.Reader, name string, size, maxSize int64) error {
	if size > maxSize {
		return fmt.Errorf("tar entry too large: %s", name)
	}

	written, err := io.CopyN(dst, src, size)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if written != size {
		return fmt.Errorf("short read for tar entry %s: got %d, want %d", name, written, size)
	}
	return nil
}

func PrepareRootfsInspection(rootfsDir string) (ext4ImagePath, issuePath string, err error) {
	ext4ImagePath, err = findExt4Image(rootfsDir)
	if err != nil {
		return "", "", err
	}

	issuePath = filepath.Join(rootfsDir, "_inspect", "etc", "issue")
	if err := os.MkdirAll(filepath.Dir(issuePath), 0o755); err != nil {
		return "", "", err
	}

	if err := extractIssueWithDebugfs(ext4ImagePath, issuePath); err != nil {
		issuePath = ""
	}
	return ext4ImagePath, issuePath, nil
}

func findExt4Image(rootfsDir string) (string, error) {
	var bestPath string
	var bestSize int64

	err := filepath.WalkDir(rootfsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".ext4") || strings.HasSuffix(name, ".img") || info.Size() > bestSize {
			bestPath = path
			bestSize = info.Size()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if bestPath == "" {
		return "", fmt.Errorf("no ext4 image found in %s", rootfsDir)
	}
	return bestPath, nil
}

func extractIssueWithDebugfs(ext4ImagePath, issuePath string) error {
	if _, err := exec.LookPath("debugfs"); err != nil {
		return err
	}

	cmd := exec.Command("debugfs", "-R", "cat /etc/issue", ext4ImagePath)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	return os.WriteFile(issuePath, out, 0o644)
}

func ComposeContainersFromManifest(projectDir, project string) ([]ContainerState, error) {
	composePath := filepath.Join(projectDir, "manifests", "docker-compose.yaml")
	b, err := os.ReadFile(composePath)
	if err != nil {
		return nil, err
	}
	env, err := loadDotEnv(filepath.Join(projectDir, "manifests", ".env"))
	if err != nil {
		return nil, err
	}
	services := parseComposeServices(string(b))
	var names []string
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	containers := make([]ContainerState, 0, len(names))
	for _, svc := range names {
		labels := services[svc]
		labelParts := []string{fmt.Sprintf("com.docker.compose.project=%s", project)}
		labelKeys := make([]string, 0, len(labels))
		for k := range labels {
			labelKeys = append(labelKeys, k)
		}
		sort.Strings(labelKeys)
		for _, key := range labelKeys {
			val := normalizeLabelValue(expandEnvRefs(labels[key], env))
			labelParts = append(labelParts, fmt.Sprintf("%s=%s", key, val))
		}

		containers = append(containers, ContainerState{
			Name:   fmt.Sprintf("%s-%s-1", project, svc),
			Labels: strings.Join(labelParts, ","),
		})
	}
	return containers, nil
}

func loadDotEnv(path string) (map[string]string, error) {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		out[key] = unquote(val)
	}
	return out, nil
}

var envRefRegex = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func expandEnvRefs(v string, env map[string]string) string {
	return envRefRegex.ReplaceAllStringFunc(v, func(ref string) string {
		m := envRefRegex.FindStringSubmatch(ref)
		if len(m) != 2 {
			return ref
		}
		if val, ok := env[m[1]]; ok {
			return val
		}
		return ref
	})
}

func normalizeLabelValue(v string) string {
	v = strings.TrimSpace(v)
	if out, err := strconv.Unquote(v); err == nil {
		return out
	}
	return strings.Trim(v, "'\"")
}

func parseComposeServices(content string) map[string]map[string]string {
	lines := strings.Split(content, "\n")
	services := map[string]map[string]string{}

	inServices := false
	currentService := ""
	inLabels := false
	labelsIndent := 0

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)

		if !inServices {
			if indent == 0 && trimmed == "services:" {
				inServices = true
			}
			continue
		}

		if indent == 0 {
			break
		}

		if indent == 2 && strings.HasSuffix(trimmed, ":") {
			currentService = strings.TrimSuffix(trimmed, ":")
			services[currentService] = map[string]string{}
			inLabels = false
			continue
		}
		if currentService == "" {
			continue
		}

		if indent == 4 && trimmed == "labels:" {
			inLabels = true
			labelsIndent = 4
			continue
		}

		if !inLabels {
			continue
		}
		if indent <= labelsIndent {
			inLabels = false
			continue
		}

		switch {
		case strings.HasPrefix(strings.TrimSpace(line), "- "):
			item := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
			k, v, ok := splitComposeLabel(item)
			if ok {
				services[currentService][k] = v
			}
		default:
			item := strings.TrimSpace(line)
			k, v, ok := splitComposeLabel(item)
			if ok {
				services[currentService][k] = v
			}
		}
	}

	return services
}

func splitComposeLabel(item string) (key, value string, ok bool) {
	if strings.Contains(item, "=") {
		parts := strings.SplitN(item, "=", 2)
		return strings.TrimSpace(unquote(parts[0])), strings.TrimSpace(unquote(parts[1])), true
	}
	if strings.Contains(item, ":") {
		parts := strings.SplitN(item, ":", 2)
		return strings.TrimSpace(unquote(parts[0])), strings.TrimSpace(unquote(parts[1])), true
	}
	return "", "", false
}

func unquote(v string) string {
	if out, err := strconv.Unquote(v); err == nil {
		return out
	}
	return strings.Trim(v, "'\"")
}

func leadingSpaces(s string) int {
	n := 0
	for _, ch := range s {
		if ch == ' ' {
			n++
			continue
		}
		break
	}
	return n
}
