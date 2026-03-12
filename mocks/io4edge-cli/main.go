package main

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ci4rail/moducop-core-api-server/mocks/mockio4edge"
)

type firmwareManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	File    string `json:"file"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "scan":
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		if err := runScan(); err != nil {
			fail(err.Error())
		}
	case "-d":
		if len(os.Args) < 4 {
			usage()
			os.Exit(2)
		}
		deviceID, ok := mockio4edge.ResolveDeviceID(os.Args[2])
		if !ok {
			fail("unknown device id: " + os.Args[2])
		}
		switch os.Args[3] {
		case "fw":
			if len(os.Args) != 4 {
				usage()
				os.Exit(2)
			}
			if err := runFw(deviceID); err != nil {
				fail(err.Error())
			}
		case "hw":
			if len(os.Args) != 4 {
				usage()
				os.Exit(2)
			}
			if err := runHW(deviceID); err != nil {
				fail(err.Error())
			}
		case "load-firmware":
			if len(os.Args) != 5 {
				usage()
				os.Exit(2)
			}
			if err := runLoadFirmware(deviceID, os.Args[4]); err != nil {
				fail(err.Error())
			}
		default:
			usage()
			os.Exit(2)
		}
	case "-i":
		if len(os.Args) != 4 || os.Args[3] != "hw" {
			usage()
			os.Exit(2)
		}
		deviceID, ok := mockio4edge.ResolveDeviceID(os.Args[2])
		if !ok {
			fail("unknown interface or device id: " + os.Args[2])
		}
		if err := runHW(deviceID); err != nil {
			fail(err.Error())
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  io4edge-cli scan")
	fmt.Fprintln(os.Stderr, "  io4edge-cli -d <device-id> fw")
	fmt.Fprintln(os.Stderr, "  io4edge-cli -d <device-id> hw")
	fmt.Fprintln(os.Stderr, "  io4edge-cli -i <interface-id|device-id> hw")
	fmt.Fprintln(os.Stderr, "  io4edge-cli -d <device-id> load-firmware <firmware.fwpkg>")
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func runFw(deviceID string) error {
	st, err := mockio4edge.LoadState()
	if err != nil {
		return err
	}
	version := st.FirmwareByDevice[deviceID]
	if version == "" {
		version = "1.0.0"
	}
	fmt.Printf("Firmware name: %s, Version %s\n", mockio4edge.FirmwareName(), version)
	return nil
}

func runHW(deviceID string) error {
	serial, ok := mockio4edge.DeviceSerial(deviceID)
	if !ok {
		return fmt.Errorf("unknown device id: %s", deviceID)
	}
	fmt.Printf("Hardware name: S100-IUO16-00-00001, rev: 0, serial: %s\n", serial)
	return nil
}

func runScan() error {
	fmt.Println("DEVICE ID               IP              HARDWARE        SERIAL")
	for _, id := range mockio4edge.DeviceIDs() {
		ip, ok := mockio4edge.DeviceIP(id)
		if !ok {
			return fmt.Errorf("missing IP for device %s", id)
		}
		serial, ok := mockio4edge.DeviceSerial(id)
		if !ok {
			return fmt.Errorf("missing serial for device %s", id)
		}
		fmt.Printf("%-23s %-15s %-15s %s\n", id, ip, "s100-iou16", serial)
	}
	return nil
}

func runLoadFirmware(deviceID, firmwarePath string) error {
	manifest, err := loadFirmwareManifest(firmwarePath)
	if err != nil {
		return err
	}

	if manifest.Version == "" {
		return errors.New("firmware version not found in package name or manifest")
	}
	version := manifest.Version

	for i := 1; i <= 10; i++ {
		fmt.Printf("Progress: %d%%\n", i*10)
		time.Sleep(1 * time.Second)
	}

	st, err := mockio4edge.LoadState()
	if err != nil {
		return err
	}
	st.FirmwareByDevice[deviceID] = version
	if err := mockio4edge.SaveState(st); err != nil {
		return err
	}

	fmt.Println("Reconnecting to restarted device.")
	fmt.Printf("Firmware name: %s, Version %s\n", mockio4edge.FirmwareName(), version)
	return nil
}

func loadFirmwareManifest(firmwarePath string) (firmwareManifest, error) {
	f, err := os.Open(firmwarePath)
	if err != nil {
		return firmwareManifest{}, err
	}
	defer func() {
		_ = f.Close()
	}()

	tr := tar.NewReader(f)
	var (
		manifestRaw []byte
		entryNames  = map[string]bool{}
		manifest    firmwareManifest
	)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return firmwareManifest{}, err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		entryNames[name] = true
		switch name {
		case "manifest.json":
			manifestRaw, err = io.ReadAll(tr)
			if err != nil {
				return firmwareManifest{}, err
			}
		}
	}
	if len(manifestRaw) == 0 {
		return firmwareManifest{}, errors.New("invalid firmware package: missing manifest.json")
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return firmwareManifest{}, fmt.Errorf("invalid firmware manifest: %w", err)
	}
	if manifest.File != "" && !entryNames[manifest.File] {
		return firmwareManifest{}, errors.New("invalid firmware package: missing firmware binary")
	}
	return manifest, nil
}
