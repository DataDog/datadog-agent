// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package kernel

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
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
const fedoraKernelModulesPath = "/usr"

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
	validationFailure
	reposDirAccessFailure
)

var errLinuxTypesHMissing = errors.New("correctly versioned kernel headers found, but linux/types.h missing")
var errReposDirInaccessible = errors.New("unable to access repos directory")

// GetKernelHeaders attempts to find kernel headers on the host, and if they cannot be found it will attempt
// to download them to headerDownloadDir
func GetKernelHeaders(headerDirs []string, headerDownloadDir, aptConfigDir, yumReposDir, zypperReposDir string) ([]string, HeaderFetchResult, error) {
	dirs, res, err := getKernelHeadersInner(headerDirs, headerDownloadDir, aptConfigDir, yumReposDir, zypperReposDir)
	if err != nil {
		return dirs, res, err
	}

	extraPath, err := createNikosExtraFiles(headerDownloadDir)
	if err != nil {
		return dirs, res, err
	}

	dirs = append(dirs, extraPath)
	return dirs, res, err
}

func getKernelHeadersInner(headerDirs []string, headerDownloadDir, aptConfigDir, yumReposDir, zypperReposDir string) ([]string, HeaderFetchResult, error) {
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

	if errors.Is(err, errLinuxTypesHMissing) {
		// If this happens, it means we've previously downloaded kernel headers containing broken
		// symlinks. We'll delete these to prevent them from affecting the next download
		log.Infof("deleting previously downloaded kernel headers")
		files, err := ioutil.ReadDir(headerDownloadDir)
		if err != nil {
			log.Warnf("error deleting kernel headers: %v", err)
		}
		for _, fi := range files {
			path := filepath.Join(headerDownloadDir, fi.Name())
			err = os.RemoveAll(path)
			if err != nil {
				log.Warnf("error deleting %s: %s", path, err)
			}
		}
	}

	d := headerDownloader{aptConfigDir, yumReposDir, zypperReposDir}
	if err = d.downloadHeaders(headerDownloadDir); err != nil {
		if errors.Is(err, errReposDirInaccessible) {
			return nil, reposDirAccessFailure, fmt.Errorf("unable to download kernel headers: %w", err)
		}
		return nil, downloadFailure, fmt.Errorf("unable to download kernel headers: %w", err)
	}

	log.Infof("successfully downloaded kernel headers to %s", headerDownloadDir)
	if err = validateHeaderDirs(hv, dirs); err == nil {
		return dirs, downloadSuccess, nil
	}
	return nil, validationFailure, fmt.Errorf("downloaded headers are not valid: %w", err)
}

// validateHeaderDirs verifies that the kernel headers in at least 1 directory matches the kernel version of the running host
// and contains the linux/types.h file
func validateHeaderDirs(hv Version, dirs []string) error {
	versionMatches, linuxTypesHFound := false, false
	for _, d := range dirs {
		if containsLinuxTypesHFile(d) {
			linuxTypesHFound = true
		}

		dirv, err := getHeaderVersion(d)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// if version.h is not found in this directory, keep going
				continue
			}
			return fmt.Errorf("error validating headers version: %w", err)
		}

		if err := compareKernelVersions(dirv, hv); err != nil {
			return err
		}

		// as long as one directory passes, validate the entire set
		versionMatches = true
	}

	if !versionMatches {
		return fmt.Errorf("no valid kernel headers found")
	}

	if !linuxTypesHFound {
		return errLinuxTypesHMissing
	}

	log.Debugf("valid kernel headers found in %v", dirs)
	return nil
}

func containsLinuxTypesHFile(dir string) bool {
	path := filepath.Join(dir, "include/linux/types.h")
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	if f != nil {
		defer f.Close()
	}
	return true
}

func getHeaderVersion(path string) (Version, error) {
	vh := filepath.Join(path, "include/generated/uapi/linux/version.h")
	f, err := os.Open(vh)
	if err != nil {
		vh = filepath.Join(path, "include/linux/version.h")
		f, err = os.Open(vh)
		if err != nil {
			return 0, err
		}
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
		fmt.Sprintf(debKernelModulesPath, hi.KernelVersion),
		fmt.Sprintf(cosKernelModulesPath, hi.KernelVersion),
		fedoraKernelModulesPath,
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

func createNikosExtraFiles(headerDownloadDir string) (string, error) {
	nikosExtraPath := filepath.Join(headerDownloadDir, "nikos-extra")
	nikosIncludePath := filepath.Join(nikosExtraPath, "include")
	_, err := createOutputDir(nikosIncludePath)
	if err != nil {
		return "", err
	}

	emptyFiles := []string{
		"asm/compiler.h",
	}

	for _, path := range emptyFiles {
		inDirPath := filepath.Join(nikosIncludePath, path)

		folderPath := filepath.Dir(inDirPath)
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			return "", err
		}

		f, err := os.Create(inDirPath)
		if err != nil {
			return "", err
		}

		if err := f.Close(); err != nil {
			return "", err
		}
	}

	return nikosExtraPath, nil
}

func compareKernelVersions(headerVersion, hostVersion Version) error {
	if headerVersion.Major() != hostVersion.Major() || headerVersion.Minor() != hostVersion.Minor() {
		return fmt.Errorf("header version %s does not match host version %s", headerVersion, hostVersion)
	}

	return nil
}
