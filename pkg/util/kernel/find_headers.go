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

	"github.com/mholt/archiver/v3"

	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const sysfsHeadersPath = "/sys/kernel/kheaders.tar.xz"
const kernelModulesPath = "/lib/modules/%s/build"
const debKernelModulesPath = "/lib/modules/%s/source"
const cosKernelModulesPath = "/usr/src/linux-headers-%s"
const centosKernelModulesPath = "/usr/src/kernels/%s"
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
	headersNotFound
)

var errReposDirInaccessible = errors.New("unable to access repos directory")

// GetKernelHeaders attempts to find kernel headers on the host, and if they cannot be found it will attempt
// to  download them to headerDownloadDir
func GetKernelHeaders(downloadEnabled bool, headerDirs []string, headerDownloadDir, aptConfigDir, yumReposDir, zypperReposDir string) ([]string, HeaderFetchResult, error) {
	hv, hvErr := HostVersion()
	if hvErr != nil {
		return nil, hostVersionErr, fmt.Errorf("unable to determine host kernel version: %w", hvErr)
	}

	if len(headerDirs) > 0 {
		if dirs := validateHeaderDirs(hv, headerDirs, true); len(dirs) > 0 {
			return headerDirs, customHeadersFound, nil
		}
		log.Debugf("unable to find configured kernel headers: no valid headers found")
	} else {
		if dirs := validateHeaderDirs(hv, getDefaultHeaderDirs(), true); len(dirs) > 0 {
			return dirs, defaultHeadersFound, nil
		}
		log.Debugf("unable to find default kernel headers: no valid headers found")

		// If no valid directories are found, attempt a fallback to extracting from `/sys/kernel/kheaders.tar.xz`
		// which is enabled via the `kheaders` kernel module and the `CONFIG_KHEADERS` kernel config option.
		// The `kheaders` module will be automatically added and removed if present and needed.
		var err error
		var dirs []string
		if dirs, err = getSysfsHeaderDirs(hv); err == nil {
			return dirs, sysfsHeadersFound, nil
		}
		log.Debugf("unable to find system kernel headers: %v", err)
	}

	downloadedDirs := validateHeaderDirs(hv, getDownloadedHeaderDirs(headerDownloadDir), false)
	if !containsCriticalHeaders(downloadedDirs) {
		// If this happens, it means we've previously downloaded kernel headers containing broken
		// symlinks. We'll delete these to prevent them from affecting the next download
		log.Infof("deleting previously downloaded kernel headers")
		for _, d := range downloadedDirs {
			deleteKernelHeaderDirectory(d)
		}
		downloadedDirs = nil
	}
	if len(downloadedDirs) > 0 {
		return downloadedDirs, downloadedHeadersFound, nil
	}
	log.Debugf("unable to find downloaded kernel headers: no valid headers found")

	if !downloadEnabled {
		return nil, headersNotFound, fmt.Errorf("no valid matching kernel header directories found")
	}

	d := headerDownloader{aptConfigDir, yumReposDir, zypperReposDir}
	if err := d.downloadHeaders(headerDownloadDir); err != nil {
		if errors.Is(err, errReposDirInaccessible) {
			return nil, reposDirAccessFailure, fmt.Errorf("unable to download kernel headers: %w", err)
		}
		return nil, downloadFailure, fmt.Errorf("unable to download kernel headers: %w", err)
	}

	log.Infof("successfully downloaded kernel headers to %s", headerDownloadDir)
	if dirs := validateHeaderDirs(hv, getDownloadedHeaderDirs(headerDownloadDir), true); len(dirs) > 0 {
		return dirs, downloadSuccess, nil
	}
	return nil, validationFailure, fmt.Errorf("downloaded headers are not valid")
}

// validateHeaderDirs checks all the given directories and returns the directories containing kernel
// headers matching the kernel version of the running host
func validateHeaderDirs(hv Version, dirs []string, checkForCriticalHeaders bool) []string {
	var valid []string
	for _, d := range dirs {
		if _, err := os.Stat(d); errors.Is(err, fs.ErrNotExist) {
			continue
		}

		dirv, err := getHeaderVersion(d)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// version.h is not found in this directory; we'll consider it valid, in case
				// it contains necessary files
				log.Debugf("found non-versioned kernel headers at %s", d)
				valid = append(valid, d)
				continue
			}
			log.Debugf("error validating %s: error validating headers version: %v", d, err)
			continue
		}

		if dirv != hv {
			log.Debugf("error validating %s: header version %s does not match host version %s", d, dirv, hv)
			continue
		}
		log.Debugf("found valid kernel headers at %s", d)
		valid = append(valid, d)
	}

	if checkForCriticalHeaders && len(valid) != 0 && !containsCriticalHeaders(valid) {
		log.Debugf("error validating %s: missing critical headers", valid)
		return nil
	}

	return valid
}

func containsCriticalHeaders(dirs []string) bool {
	criticalPaths := []string{
		"include/linux/types.h",
		"include/linux/kconfig.h",
	}

	searchResult := make(map[string]bool)
	for _, path := range criticalPaths {
		searchResult[path] = false
	}

	for _, criticalPath := range criticalPaths {
		for _, dir := range dirs {
			path := filepath.Join(dir, criticalPath)
			_, err := os.Stat(path)
			if !errors.Is(err, fs.ErrNotExist) {
				searchResult[criticalPath] = true
			}
		}
	}

	for _, found := range searchResult {
		if !found {
			return false
		}
	}

	return true
}

func deleteKernelHeaderDirectory(dir string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Warnf("error deleting kernel headers: %v", err)
	}
	for _, fi := range files {
		path := filepath.Join(dir, fi.Name())
		err = os.RemoveAll(path)
		if err != nil {
			log.Warnf("error deleting %s: %s", path, err)
		}
	}
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
		fmt.Sprintf(centosKernelModulesPath, hi.KernelVersion),
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
		log.Debugf("found valid kernel headers at %s", tmpPath)
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
