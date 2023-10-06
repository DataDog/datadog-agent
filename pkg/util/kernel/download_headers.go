// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/avast/retry-go/v4"

	"github.com/DataDog/nikos/apt"
	"github.com/DataDog/nikos/cos"
	"github.com/DataDog/nikos/rpm"
	"github.com/DataDog/nikos/types"
	"github.com/DataDog/nikos/wsl"
)

// customLogger is a wrapper around our logging utility which allows nikos to use our logging functions
type customLogger struct{}

func (c customLogger) Debug(args ...interface{})                 { log.Debug(args...) }
func (c customLogger) Info(args ...interface{})                  { log.Info(args...) }
func (c customLogger) Warn(args ...interface{})                  { log.Warn(args...) }
func (c customLogger) Error(args ...interface{})                 { log.Error(args...) }
func (c customLogger) Debugf(format string, args ...interface{}) { log.Debugf(format, args...) }
func (c customLogger) Infof(format string, args ...interface{})  { log.Infof(format, args...) }
func (c customLogger) Warnf(format string, args ...interface{})  { log.Warnf(format, args...) }
func (c customLogger) Errorf(format string, args ...interface{}) { log.Errorf(format, args...) }

var _ types.Logger = customLogger{}

type headerDownloader struct {
	aptConfigDir   string
	yumReposDir    string
	zypperReposDir string
}

// downloadHeaders attempts to download kernel headers & place them in headerDownloadDir
func (h *headerDownloader) downloadHeaders(headerDownloadDir string) error {
	var (
		target    types.Target
		backend   types.Backend
		outputDir string
		err       error
	)

	if outputDir, err = createOutputDir(headerDownloadDir); err != nil {
		return fmt.Errorf("unable create output directory %s: %s", headerDownloadDir, err)
	}

	if target, err = types.NewTarget(); err != nil {
		return fmt.Errorf("failed to retrieve target information: %s", err)
	}

	log.Infof("Downloading kernel headers for target distribution %s, release %s, kernel %s",
		target.Distro.Display,
		target.Distro.Release,
		target.Uname.Kernel,
	)
	log.Debugf("Target OSRelease: %s", target.OSRelease)

	var reposDir string
	if reposDir, err = h.verifyReposDir(target); err != nil {
		return err
	}

	if backend, err = h.getHeaderDownloadBackend(&target, reposDir); err != nil {
		return fmt.Errorf("unable to get kernel header download backend: %s", err)
	}
	defer backend.Close()

	return retry.Do(func() error {
		if err := backend.GetKernelHeaders(outputDir); err != nil {
			return fmt.Errorf("failed to download kernel headers: %s", err)
		}
		return nil
	}, retry.Attempts(2), retry.Delay(5*time.Second), retry.OnRetry(func(_ uint, err error) {
		log.Infof("%s. Waiting 5 seconds and retrying kernel header download.", err)
	}))
}

func (h *headerDownloader) verifyReposDir(target types.Target) (string, error) {
	var reposDir string
	switch strings.ToLower(target.Distro.Display) {
	case "fedora", "rhel", "redhat", "centos", "amazon", "oracle":
		reposDir = h.yumReposDir
	case "opensuse", "opensuse-leap", "opensuse-tumbleweed", "opensuse-tumbleweed-kubic", "suse", "sles", "sled", "caasp":
		reposDir = h.zypperReposDir
	case "debian", "ubuntu":
		reposDir = h.aptConfigDir
	default:
		// any other distro doesn't need a repos dir, so we can return the zero value
		return reposDir, nil
	}

	if _, err := os.Stat(reposDir); err != nil {
		log.Warnf("Unable to read %v, which is necessary for downloading kernel headers. If you are in a "+
			"containerized environment, please ensure this directory is mounted.", reposDir)
		return reposDir, errReposDirInaccessible
	}
	return reposDir, nil
}

func (h *headerDownloader) getHeaderDownloadBackend(target *types.Target, reposDir string) (backend types.Backend, err error) {
	logger := customLogger{}
	switch strings.ToLower(target.Distro.Display) {
	case "fedora":
		backend, err = rpm.NewFedoraBackend(target, reposDir, logger)
	case "rhel", "redhat":
		backend, err = rpm.NewRedHatBackend(target, reposDir, logger)
	case "oracle":
		backend, err = rpm.NewOracleBackend(target, reposDir, logger)
	case "amazon":
		if target.Distro.Release == "2022" {
			backend, err = rpm.NewAmazonLinux2022Backend(target, reposDir, logger)
		} else {
			backend, err = rpm.NewRedHatBackend(target, reposDir, logger)
		}
	case "centos":
		backend, err = rpm.NewCentOSBackend(target, reposDir, logger)
	case "opensuse", "opensuse-leap", "opensuse-tumbleweed", "opensuse-tumbleweed-kubic":
		backend, err = rpm.NewOpenSUSEBackend(target, reposDir, logger)
	case "suse", "sles", "sled", "caasp":
		backend, err = rpm.NewSLESBackend(target, reposDir, logger)
	case "debian", "ubuntu":
		backend, err = apt.NewBackend(target, reposDir, logger)
	case "cos":
		backend, err = cos.NewBackend(target, logger)
	case "wsl":
		backend, err = wsl.NewBackend(target, logger)
	default:
		err = fmt.Errorf("unsupported distribution '%s'", target.Distro.Display)
	}
	return
}

func createOutputDir(path string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("unable to get absolute path: %s", err)
	}

	err = os.MkdirAll(absolutePath, 0755)
	return absolutePath, err
}
