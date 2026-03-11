package cpumanager

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/ci4rail/moducop-core-api-server/internal/prefixfs"
)

const (
	issueFilePath = "/etc/issue"
	menderAppRootDir = "/data/mender-app"
)

// coreOSVersionFromTargetFS reads the /etc/issue file from the target filesystem and
// extracts the CoreOS version information.
// It expect in the second line of the file a string like
// "Moducop-CPU01_Standard-Image_v2.6.0.f457f6d.20260210.1540".
// The name is aligned to the format contained in the artifact_provides field "rootfs-image.version".
// e.g. name="cpu01-standard" version="v2.6.0.f457f6d.20260210.1540".
// Returns name, version, error
func coreOSVersionFromTargetFS() (string, string, error) {
	// read /etc/issue from the target filesystem and extract the CoreOS version
	issueFile := prefixfs.Path(issueFilePath)
	data, err := os.ReadFile(issueFile)
	if err != nil {
		return "", "", fmt.Errorf("read /etc/issue: %w", err)
	}
	// read second line
	lines := string(data)
	lineList := strings.Split(lines, "\n")
	if len(lineList) < 2 {
		return "", "", fmt.Errorf("unexpected format of /etc/issue: less than 2 lines")
	}
	return coreOsVersionFromIssueLine(lineList[1])
}

func coreOsVersionFromIssueLine(line string) (string, string, error) {
	// extract name and version from a string like "Moducop-CPU01_Standard-Image_v2.6.0.f457f6d.20260210.1540"
	re := regexp.MustCompile(`^[A-Za-z0-9]+-(?P<name>.+)-Image_(?P<version>v\d+\.\d+\.\d+(?:\..+)?)$`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return "", "", fmt.Errorf("invalid format for /etc/issue line: %s", line)
	}
	if len(matches) != 3 {
		return "", "", fmt.Errorf("unexpected regex match groups: %v", matches)
	}
	name := matches[1]
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	version := matches[2]
	return name, version, nil
}

// appVersionFromTargetFS reads the version of the given application from the target filesystem.
// It expects a file at /data/mender-app/<appName>/manifest/.env file containing the version string as
// a shell variable like "SOFTWARE_VERSION=<version>".
// Returns version, error
func appVersionFromTargetFS(appName string) (string, error) {
	// read version from /data/mender-app/<appName>/manifest/.env
	envFile := fmt.Sprintf("%s/%s/manifest/.env", menderAppRootDir, appName)
	data, err := os.ReadFile(prefixfs.Path(envFile))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", envFile, err)
	}
	return appVersionFromData(string(data))
}

func appVersionFromData(data string) (string, error) {
	lines := string(data)
	lineList := strings.Split(lines, "\n")
	re := regexp.MustCompile(`^SOFTWARE_VERSION=(?P<version>.+)$`)

	for _, line := range lineList {
		matches := re.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		version := matches[1]
		return version, nil
	}
	return "", fmt.Errorf("invalid format for .env data")
}

func listApplicationsFromTargetFS() ([]string, error) {
	// list directories in /data/mender-app. Exclude directories ending with -previous
	appsDir := prefixfs.Path(menderAppRootDir)
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return nil, fmt.Errorf("read applications directory: %w", err)
	}
	var apps []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasSuffix(entry.Name(), "-previous") {
			apps = append(apps, entry.Name())
		}
	}
	return apps, nil
}