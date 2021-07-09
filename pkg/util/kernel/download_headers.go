// +build linux

package kernel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

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

// downloadHeaders attempts to download kernel headers & place them in headerDownloadDir
func downloadHeaders(headerDownloadDir string) error {
	var (
		target    types.Target
		backend   types.Backend
		outputDir string
		err       error
	)

	if outputDir, err = createOutputDir(headerDownloadDir); err != nil {
		return fmt.Errorf("unable create output directory %s: %s", headerDownloadDir, err)
	}

	if target, err = getHeaderDownloadTarget(); err != nil {
		return fmt.Errorf("failed to retrieve target information: %s", err)
	}

	log.Infof("Downloading kernel headers for target distribution %s, release %s, kernel %s",
		target.Distro.Display,
		target.Distro.Release,
		target.Uname.Kernel,
	)
	log.Debugf("Target OSRelease: %s", target.OSRelease)

	if backend, err = getHeaderDownloadBackend(&target); err != nil {
		return fmt.Errorf("unable to get kernel header download backend: %s", err)
	}

	if err = backend.GetKernelHeaders(outputDir); err != nil {
		return fmt.Errorf("failed to download kernel headers: %s", err)
	}

	return nil
}

func getHeaderDownloadTarget() (types.Target, error) {
	target, err := types.NewTarget()
	if err != nil {
		return types.Target{}, err
	}

	if _, err := os.Stat("/run/WSL"); err == nil {
		target.Distro.Display = "wsl"
	} else if id := target.OSRelease["ID"]; target.Distro.Display == "" && id != "" {
		target.Distro.Display = id
	}

	return target, nil
}

func getHeaderDownloadBackend(target *types.Target) (backend types.Backend, err error) {
	logger := customLogger{}
	switch strings.ToLower(target.Distro.Display) {
	case "fedora", "rhel":
		backend, err = rpm.NewRedHatBackend(target, "/etc/yum.repos.d", logger)
	case "centos":
		backend, err = rpm.NewCentOSBackend(target, "/etc/yum.repos.d", logger)
	case "opensuse":
		backend, err = rpm.NewOpenSUSEBackend(target, "/etc/zypp/repos.d", logger)
	case "sle":
		backend, err = rpm.NewSLESBackend(target, "/etc/zypp/repos.d", logger)
	case "debian", "ubuntu":
		backend, err = apt.NewBackend(target, "/etc/apt", logger)
	case "cos":
		backend, err = cos.NewBackend(target, logger)
	case "wsl":
		backend, err = wsl.NewBackend(target, logger)
	default:
		err = fmt.Errorf("Unsupported distribution '%s'", target.Distro.Display)
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
