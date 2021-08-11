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
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/mholt/archiver/v3"
)

const sysfsHeadersPath = "/sys/kernel/kheaders.tar.xz"
const kernelModulesPath = "/lib/modules/%s/build"
const debKernelModulesPath = "/lib/modules/%s/source"
const cosKernelModulesPath = "/usr/src/linux-headers-%s"

var versionCodeRegexp = regexp.MustCompile(`^#define[\t ]+LINUX_VERSION_CODE[\t ]+(\d+)$`)

// HeaderFetchResult enumerates kernel header fetching success & failure modes
type HeaderFetchResult int

const (
	// NotAttempted represents the case where runtime compilation fails prior to attempting to
	// fetch kernel headers
	NotAttempted HeaderFetchResult = iota
	customHeadersFound
	defaultHeadersFound
	sysfsHeadersFound
	downloadedHeadersFound
	downloadSuccess
	hostVersionErr
	downloadFailure
)

// GetKernelHeaders attempts to find kernel headers on the host, and if they cannot be found it will attempt
// to  download them to headerDownloadDir
func GetKernelHeaders(headerDirs []string, headerDownloadDir, aptConfigDir, yumReposDir, zypperReposDir string) ([]string, HeaderFetchResult, error) {
	hv, hvErr := HostVersion()
	if hvErr != nil {
		return nil, hostVersionErr, fmt.Errorf("unable to determine host kernel version: %w", hvErr)
	}

	var err error
	var dirs []string
	if len(headerDirs) > 0 {
		if err = validateHeaderDirs(hv, headerDirs); err == nil {
			return headerDirs, customHeadersFound, nil
		}
		log.Debugf("unable to find configured kernel headers: %s", err)
	} else {
		dirs = getDefaultHeaderDirs()
		if err = validateHeaderDirs(hv, dirs); err == nil {
			return dirs, defaultHeadersFound, nil
		}
		log.Debugf("unable to find default kernel headers: %s", err)

		// If no valid directories are found, attempt a fallback to extracting from `/sys/kernel/kheaders.tar.xz`
		// which is enabled via the `kheaders` kernel module and the `CONFIG_KHEADERS` kernel config option.
		// The `kheaders` module will be automatically added and removed if present and needed.
		if dirs, err = getSysfsHeaderDirs(hv); err == nil {
			return dirs, sysfsHeadersFound, nil
		}
		log.Debugf("unable to find system kernel headers: %s", err)
	}

	dirs = getDownloadedHeaderDirs(headerDownloadDir)
	if err = validateHeaderDirs(hv, dirs); err == nil {
		return dirs, downloadedHeadersFound, nil
	}
	log.Debugf("unable to find downloaded kernel headers: %s", err)

	d := headerDownloader{aptConfigDir, yumReposDir, zypperReposDir}
	if err = d.downloadHeaders(headerDownloadDir); err == nil {
		log.Infof("successfully downloaded kernel headers to %s", dirs)
		if err = validateHeaderDirs(hv, dirs); err == nil {
			return dirs, downloadSuccess, nil
		}
		return nil, downloadFailure, fmt.Errorf("downloaded headers are not valid: %w", err)
	}
	return nil, downloadFailure, fmt.Errorf("unable to download kernel headers: %w", err)
}

// validateHeaderDirs verifies that the kernel headers in at least 1 directory matches the kernel version of the running host
func validateHeaderDirs(hv Version, dirs []string) error {
	for _, d := range dirs {
		dirv, err := getHeaderVersion(d)
		if err != nil {
			if os.IsNotExist(err) {
				// if version.h is not found in this directory, keep going
				continue
			}
			return fmt.Errorf("error validating headers version: %w", err)
		}
		if dirv != hv {
			return fmt.Errorf("header version %s does not match host version %s", dirv, hv)
		}
		// as long as one directory passes, validate the entire set
		log.Debugf("found valid kernel headers at %s", d)
		return nil
	}

	return fmt.Errorf("no valid kernel headers found")
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

func getDefaultHeaderDirs() []string {
	// KernelVersion == uname -r
	hi := host.GetStatusInformation()
	if hi.KernelVersion == "" {
		return []string{}
	}

	dirs := []string{
		fmt.Sprintf(kernelModulesPath, hi.KernelVersion),
	}
	switch hi.Platform {
	case "debian":
		dirs = append(dirs, fmt.Sprintf(debKernelModulesPath, hi.KernelVersion))
	case "cos":
		dirs = append(dirs, fmt.Sprintf(cosKernelModulesPath, hi.KernelVersion))
	}
	return dirs
}

func getDownloadedHeaderDirs(headerDownloadDir string) []string {
	dirs := getDefaultHeaderDirs()
	for i, dir := range dirs {
		dirs[i] = filepath.Join(headerDownloadDir, dir)
	}

	return dirs
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
	log.Debugf("found valid kernel headers at %s", tmpPath)
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
