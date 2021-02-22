// +build linux

package kernel

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/mholt/archiver/v3"
)

const sysfsHeadersPath = "/sys/kernel/kheaders.tar.xz"
const kernelModulesPath = "/lib/modules/%s/build"

var versionCodeRegexp = regexp.MustCompile(`^#define[\t ]+LINUX_VERSION_CODE[\t ]+(\d+)$`)

// FindHeaderDirs finds kernel header base directories that contain a `LINUX_VERSION_CODE` matching the running kernel.
// If no directories are found, it will attempt a fallback to extracting from `/sys/kernel/kheaders.tar.xz`
// which is enabled via the `kheaders` kernel module and the `CONFIG_KHEADERS` kernel config option.
// The `kheaders` module will be automatically added and removed if present and needed.
func FindHeaderDirs() ([]string, error) {
	hv, err := HostVersion()
	if err != nil {
		return nil, fmt.Errorf("unable to determine host kernel version: %w", err)
	}

	dir, err := getHeaderDirs(hv)
	if err == nil {
		return dir, nil
	}

	if os.IsNotExist(err) {
		// as last resort, look for sysfs headers
		if dir, syserr := getSysfsHeaderDirs(hv); syserr == nil {
			return dir, nil
		}
	}
	return nil, err
}

// ValidateHeaderDir verifies that the kernel headers in the directory match the kernel version of the running host
func ValidateHeaderDir(path string) error {
	hv, err := HostVersion()
	if err != nil {
		return err
	}

	dirv, err := getHeaderVersion(path)
	if err != nil {
		return err
	}

	if dirv != hv {
		return fmt.Errorf("header version %s does not match host version %s", dirv, hv)
	}
	return nil
}

func getHeaderDirs(v Version) ([]string, error) {
	hi := host.GetStatusInformation()
	if hi.KernelVersion == "" {
		return nil, fmt.Errorf("unable to get host metadata")
	}

	// KernelVersion == uname -r
	dirs := []string{fmt.Sprintf(kernelModulesPath, hi.KernelVersion)}
	if hi.Platform == "debian" {
		dirs = append(dirs, fmt.Sprintf("/lib/modules/%s/source", hi.KernelVersion))
	}

	for _, d := range dirs {
		hv, err := getHeaderVersion(d)
		if err != nil {
			// if version.h is not found in this directory, keep going
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("error validating headers version: %w", err)
		}
		if hv == v {
			return dirs, nil
		}
	}
	return nil, os.ErrNotExist
}

func getHeaderVersion(path string) (Version, error) {
	vh := filepath.Join(path, "include/generated/uapi/linux/version.h")
	f, err := os.Open(vh)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return parseHeaderVersion(f)
}

func parseHeaderVersion(r io.Reader) (Version, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if matches := versionCodeRegexp.FindSubmatch(scanner.Bytes()); matches != nil {
			code, err := strconv.ParseUint(string(matches[1]), 10, 32)
			if err != nil {
				continue
			}
			return Version(code), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("no kernel version found")
}

func getSysfsHeaderDirs(v Version) ([]string, error) {
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("linux-headers-%s", v))
	fi, err := os.Stat(tmpPath)
	if err == nil && fi.IsDir() {
		hv, err := getHeaderVersion(tmpPath)
		if err != nil {
			// remove tmp dir if it errors
			_ = os.RemoveAll(tmpPath)
			return nil, fmt.Errorf("unable to verify headers version: %w", err)
		}
		if hv != v {
			// remove tmp dir if it fails to validate
			_ = os.RemoveAll(tmpPath)
			return nil, fmt.Errorf("header version %s does not match expected host version %s", v, hv)
		}
		return []string{tmpPath}, nil
	}

	if !sysfsHeadersExist() {
		if err := loadKHeadersModule(); err != nil {
			return nil, err
		}
		defer func() { _ = unloadKHeadersModule() }()
		if !sysfsHeadersExist() {
			return nil, fmt.Errorf("unable to find sysfs kernel headers")
		}
	}

	txz := archiver.NewTarXz()
	if err = txz.Unarchive(sysfsHeadersPath, tmpPath); err != nil {
		return nil, fmt.Errorf("unable to extract kernel headers: %w", err)
	}
	return []string{tmpPath}, nil
}

func sysfsHeadersExist() bool {
	_, err := os.Stat(sysfsHeadersPath)
	// if any kind of error (including not exists), treat as not being there
	return err == nil
}

func loadKHeadersModule() error {
	cmd := exec.Command("modprobe", "kheaders")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil || stderr.Len() > 0 {
		return fmt.Errorf("unable to load kheaders module: %s", stderr.String())
	}
	return nil
}

func unloadKHeadersModule() error {
	cmd := exec.Command("rmmod", "kheaders")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil || stderr.Len() > 0 {
		return fmt.Errorf("unable to unload kheaders module: %s", stderr.String())
	}
	return nil
}
