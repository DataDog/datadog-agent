// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

// Package headers is utilities for downloading Linux kernel headers
package headers

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/archive"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var headerTelemetry = struct {
	success telemetry.Counter
	error   telemetry.Counter
}{
	success: telemetry.NewCounter("ebpf__runtime_compilation__header_download", "success", []string{"platform", "platform_version", "kernel", "arch", "result"}, "count of kernel header download successes"),
	error:   telemetry.NewCounter("ebpf__runtime_compilation__header_download", "error", []string{"platform", "platform_version", "kernel", "arch", "result"}, "count of kernel header download errors"),
}

const sysfsHeadersPath = "/sys/kernel/kheaders.tar.xz"
const kernelModulesPath = "/lib/modules/%s/build"
const debKernelModulesPath = "/lib/modules/%s/source"
const cosKernelModulesPath = "/usr/src/linux-headers-%s"
const centosKernelModulesPath = "/usr/src/kernels/%s"

var versionCodeRegexp = regexp.MustCompile(`^#define[\t ]+LINUX_VERSION_CODE[\t ]+(\d+)$`)

var errReposDirInaccessible = errors.New("unable to access repos directory")

// Copied from https://github.com/DataDog/agent-payload/blob/master/process/connections.pb.go
// to avoid CGO dependency
var kernelHeaderFetchResultName = map[int]string{
	0:  "FetchNotAttempted",
	1:  "CustomHeadersFound",
	2:  "DefaultHeadersFound",
	3:  "SysfsHeadersFound",
	4:  "DownloadedHeadersFound",
	5:  "DownloadSuccess",
	6:  "HostVersionErr",
	7:  "DownloadFailure",
	8:  "ValidationFailure",
	9:  "ReposDirAccessFailure",
	10: "HeadersNotFoundDownloadDisabled",
}

type headerFetchResult int

const (
	notAttempted headerFetchResult = iota
	customHeadersFound
	defaultHeadersFound
	sysfsHeadersFound
	downloadedHeadersFound
	downloadSuccess
	hostVersionErr
	downloadFailure
	validationFailure
	reposDirAccessFailure
	headersNotFoundDownloadDisabled
)

// IsSuccess returns whether the headers were fetched successfully
func (r headerFetchResult) IsSuccess() bool {
	return customHeadersFound <= r && r <= downloadSuccess
}

// HeaderProvider is a global object which is responsible for managing the fetching of kernel headers
var HeaderProvider *headerProvider
var providerMu sync.Mutex

type headerProvider struct {
	downloadEnabled   bool
	headerDirs        []string
	headerDownloadDir string

	downloader headerDownloader

	result        headerFetchResult
	kernelHeaders []string
}

// HeaderOptions are options for the kernel header download process
type HeaderOptions struct {
	DownloadEnabled bool
	Dirs            []string
	DownloadDir     string

	AptConfigDir   string
	YumReposDir    string
	ZypperReposDir string
}

func initProvider(opts HeaderOptions) {
	HeaderProvider = &headerProvider{
		downloadEnabled:   opts.DownloadEnabled,
		headerDirs:        opts.Dirs,
		headerDownloadDir: opts.DownloadDir,

		downloader: headerDownloader{
			aptConfigDir:   opts.AptConfigDir,
			yumReposDir:    opts.YumReposDir,
			zypperReposDir: opts.ZypperReposDir,
		},

		result:        notAttempted,
		kernelHeaders: []string{},
	}
}

// GetKernelHeaders fetches and returns kernel headers for the currently running kernel.
//
// The first time GetKernelHeaders is called, it will search the host for kernel headers, and if they
// cannot be found it will attempt to download headers to the configured header download directory.
//
// Any subsequent calls to GetKernelHeaders will return the result of the first call. This is because
// kernel header downloading can be a resource intensive process, so we don't want to retry it an unlimited
// number of times.
func GetKernelHeaders(opts HeaderOptions) []string {
	providerMu.Lock()
	defer providerMu.Unlock()

	if HeaderProvider == nil {
		initProvider(opts)
	}

	if HeaderProvider.result != notAttempted {
		log.Debugf("kernel headers requested: returning result of previous search")
		return HeaderProvider.kernelHeaders
	}

	hv, err := kernel.HostVersion()
	if err != nil {
		HeaderProvider.result = hostVersionErr
		log.Warnf("Unable to find kernel headers: unable to determine host kernel version: %s", err)
		return []string{}
	}

	headers, result, err := HeaderProvider.getKernelHeaders(hv)
	submitTelemetry(result)

	HeaderProvider.kernelHeaders = headers
	HeaderProvider.result = result
	if err != nil {
		log.Warnf("Unable to find kernel headers: %s", err)
	}

	return headers
}

// GetResult returns the result of kernel header fetching
func (h *headerProvider) GetResult() headerFetchResult {
	providerMu.Lock()
	defer providerMu.Unlock()

	if h == nil {
		return notAttempted
	}

	return h.result
}

func (h *headerProvider) getKernelHeaders(hv kernel.Version) ([]string, headerFetchResult, error) {
	log.Debugf("beginning search for kernel headers")

	if len(h.headerDirs) > 0 {
		if dirs := validateHeaderDirs(hv, h.headerDirs, true); len(dirs) > 0 {
			return h.headerDirs, customHeadersFound, nil
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

	downloadedDirs := validateHeaderDirs(hv, getDownloadedHeaderDirs(h.headerDownloadDir), false)
	if len(downloadedDirs) > 0 && !containsCriticalHeaders(downloadedDirs) {
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

	if !h.downloadEnabled {
		return nil, headersNotFoundDownloadDisabled, errors.New("no valid matching kernel header directories found. To download kernel headers, set system_probe_config.enable_kernel_header_download to true")
	}

	return h.downloadHeaders(hv)
}

func (h *headerProvider) downloadHeaders(hv kernel.Version) ([]string, headerFetchResult, error) {
	if err := h.downloader.downloadHeaders(h.headerDownloadDir); err != nil {
		if errors.Is(err, errReposDirInaccessible) {
			return nil, reposDirAccessFailure, fmt.Errorf("unable to download kernel headers: %w", err)
		}
		return nil, downloadFailure, fmt.Errorf("unable to download kernel headers: %w", err)
	}

	log.Infof("successfully downloaded kernel headers to %s", h.headerDownloadDir)
	if dirs := validateHeaderDirs(hv, getDownloadedHeaderDirs(h.headerDownloadDir), true); len(dirs) > 0 {
		return dirs, downloadSuccess, nil
	}
	return nil, validationFailure, errors.New("downloaded headers are not valid")
}

// validateHeaderDirs checks all the given directories and returns the directories containing kernel
// headers matching the kernel version of the running host
func validateHeaderDirs(hv kernel.Version, dirs []string, checkForCriticalHeaders bool) []string {
	valid := make(map[string]struct{})
	for _, rd := range dirs {
		if _, err := os.Stat(rd); errors.Is(err, fs.ErrNotExist) {
			continue
		}

		d, err := filepath.EvalSymlinks(rd)
		if err != nil {
			log.Debugf("unable to eval symlink for %s: %s", rd, err)
			continue
		}
		log.Debugf("resolved header dir %s to %s", rd, d)
		if _, ok := valid[d]; ok {
			continue
		}

		dirv, err := getHeaderVersion(d)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// version.h is not found in this directory; we'll consider it valid, in case
				// it contains necessary files
				log.Debugf("found non-versioned kernel headers at %s", d)
				valid[d] = struct{}{}
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
		valid[d] = struct{}{}
	}

	dirlist := slices.Collect(maps.Keys(valid))
	if checkForCriticalHeaders && len(dirlist) != 0 && !containsCriticalHeaders(dirlist) {
		log.Debugf("error validating %s: missing critical headers", dirlist)
		return nil
	}
	return dirlist
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
	files, err := os.ReadDir(dir)
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

func getHeaderVersion(path string) (kernel.Version, error) {
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

func parseHeaderVersion(r io.Reader) (kernel.Version, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if matches := versionCodeRegexp.FindSubmatch(scanner.Bytes()); matches != nil {
			code, err := strconv.ParseUint(string(matches[1]), 10, 32)
			if err != nil {
				continue
			}
			return kernel.Version(code), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("no kernel version found")
}

func getDefaultHeaderDirs() []string {
	hi, err := kernel.Release()
	if err != nil {
		return []string{}
	}

	return []string{
		fmt.Sprintf(kernelModulesPath, hi),
		fmt.Sprintf(debKernelModulesPath, hi),
		fmt.Sprintf(cosKernelModulesPath, hi),
		fmt.Sprintf(centosKernelModulesPath, hi),
	}
}

func getDownloadedHeaderDirs(headerDownloadDir string) []string {
	dirs := getDefaultHeaderDirs()
	for i, dir := range dirs {
		dirs[i] = filepath.Join(headerDownloadDir, dir)
	}

	return dirs
}

func getSysfsHeaderDirs(v kernel.Version) ([]string, error) {
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
			return nil, errors.New("unable to find sysfs kernel headers")
		}
	}

	if err = archive.TarXZExtractAll(sysfsHeadersPath, tmpPath); err != nil {
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

func submitTelemetry(result headerFetchResult) {
	if result == notAttempted {
		return
	}

	platform, err := kernel.Platform()
	if err != nil {
		log.Warnf("failed to retrieve host platform information: %s", err)
		return
	}
	platformVersion, err := kernel.PlatformVersion()
	if err != nil {
		log.Warnf("failed to get platform version: %s", err)
		return
	}
	kernelVersion, err := kernel.Release()
	if err != nil {
		log.Warnf("failed to get kernel version: %s", err)
		return
	}
	arch, err := kernel.Machine()
	if err != nil {
		log.Warnf("failed to get kernel architecture: %s", err)
		return
	}

	tags := []string{
		platform,
		platformVersion,
		kernelVersion,
		arch,
		kernelHeaderFetchResultName[int(result)],
	}

	if result.IsSuccess() {
		headerTelemetry.success.Inc(tags...)
	} else {
		headerTelemetry.error.Inc(tags...)
	}
}
