package mockmender

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type HeaderInfo struct {
	Payloads []struct {
		Type string `json:"type"`
	} `json:"payloads"`
	ArtifactDepends struct {
		DeviceType []string `json:"device_type"`
	} `json:"artifact_depends"`
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
			gz, zErr := gzip.NewReader(bytes.NewReader(b))
			if zErr != nil {
				return info, "", fmt.Errorf("open header.tar.gz: %w", zErr)
			}
			defer gz.Close()
			parsed, pErr := parseHeaderTar(gz)
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
		parsed, pErr := parseHeaderTar(bytes.NewReader(headerTar))
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

func parseHeaderTar(r io.Reader) (HeaderInfo, error) {
	var info HeaderInfo
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return info, fmt.Errorf("read header tar: %w", err)
		}
		if h.Name == "header-info" {
			b, rErr := io.ReadAll(tr)
			if rErr != nil {
				return info, fmt.Errorf("read header-info: %w", rErr)
			}
			if uErr := json.Unmarshal(b, &info); uErr != nil {
				return info, fmt.Errorf("parse header-info: %w", uErr)
			}
			return info, nil
		}
	}
	return info, fmt.Errorf("header-info not found")
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
			if _, err := io.Copy(f, tr); err != nil {
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
