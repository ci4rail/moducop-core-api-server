package updatestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ci4rail/moducop-core-api-server/internal/loglite"
	"github.com/ci4rail/moducop-core-api-server/internal/prefixfs"
)

const (
	updateFilePath = "/data/core-api-server/updates/"
	dirModeDefault = 0o755
)

var errRemoveFiles = errors.New("failed to remove files")

func EnsurePath() error {
	return os.MkdirAll(GetPath(), dirModeDefault)
}

func GetPath() string {
	return prefixfs.Path(updateFilePath)
}

// remove all files matching the pattern in the update directory, except those in the excludedFiles list
func Clean(logger *loglite.Logger, filePattern string, excludedFiles []string) error {
	files, err := filepath.Glob(filepath.Join(GetPath(), filePattern))
	if err != nil {
		return err
	}
	excludeMap := make(map[string]struct{}, len(excludedFiles))
	for _, f := range excludedFiles {
		excludeMap[filepath.Base(f)] = struct{}{}
	}
	errCount := 0
	for _, f := range files {
		if _, excluded := excludeMap[filepath.Base(f)]; !excluded {
			logger.Infof("Removing %s", f)
			if err := os.Remove(f); err != nil {
				logger.Errorf("Failed to remove %s: %v", f, err)
				errCount++
				continue
			}
		}
	}
	if errCount > 0 {
		return fmt.Errorf("%w: %d", errRemoveFiles, errCount)
	}
	return nil
}
