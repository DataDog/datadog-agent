package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/rc-update-client/pkg/catalog"
	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"

	// "github.com/DataDog/datadog-agent/pkg/config"

	"golang.org/x/sys/unix"
)

const (
	requiredMinimalBytes = 1024 * 1024 * 1024 * 2 // 2GB
	unpackPath           = "unpack"
)

type Config struct {
	Agent        Version `json:"agent"`
	Experimental Version `json:"experimental"`
}

type Version struct {
	Version string `json:"version"`
}

type Updater struct {
	rcService *service.Service
	rcClient  *remote.Client

	binariesPath string
	ctx          context.Context

	updateChan chan<- Config
}

func (u *Updater) Start() error {
	err := u.prerequisites()
	if err != nil {
		return err
	}
	u.rcService.Start(context.Background())
	return nil
}

func (u *Updater) Close() {
	u.rcService.Stop()
}

func NewUpdater(
	ctx context.Context,
	rcService *service.Service,
	binariesPath string,
	updateChan chan<- Config,
) (*Updater, error) {
	rcClient, err := remote.NewClient(
		"agent-updater",
		rcService,
		"7.50.0",
		[]data.Product{data.ProductUpdaterData},
		10*time.Second,
	)
	if err != nil {
		return nil, err
	}
	rcClient.Start()

	updater := &Updater{
		rcService:    rcService,
		binariesPath: binariesPath,
		ctx:          ctx,
		rcClient:     rcClient,
		updateChan:   updateChan,
	}
	rcClient.Subscribe(data.ProductUpdaterData, updater.updateNotif)
	return updater, nil
}

func (u *Updater) updateNotif(
	configs map[string]state.RawConfig,
	applyStateCallback func(string, state.ApplyStatus),
) {
	if len(configs) == 0 {
		log.Printf("Configuration not Ready yet")
		return
	}

	var versionConf Config
	for k, v := range configs {
		if v.Metadata.ID == "default" {
			err := json.Unmarshal(v.Config, &versionConf)
			if err != nil {
				applyStateCallback(k, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: err.Error(),
				})
				return
			} else {
				u.updateChan <- versionConf
				applyStateCallback(k, state.ApplyStatus{
					State: state.ApplyStateAcknowledged,
				})
			}
		}
	}
}

// TODO check also permissions and so on
func (u *Updater) prerequisites() error {
	info, err := os.Stat(u.binariesPath)
	if err == nil && !info.IsDir() {
		return fmt.Errorf("Target storage path it's a file")
	}

	// if it failed then we just make the whole path or give up
	if err != nil {
		log.Println("Create missing storage path")
		err = os.MkdirAll(u.binariesPath, os.ModePerm)
		if err != nil {
			return err
		}
	}

	// Available blocks * size per block = available space in bytes
	var stat unix.Statfs_t
	unix.Statfs(u.binariesPath, &stat)
	if stat.Bavail*uint64(stat.Bsize) < requiredMinimalBytes {
		return fmt.Errorf("Not enough disk space to perform an update")
	}
	log.Println("Enough disk space detected in update path")

	return nil
}

// TODO should also check the content of the folders
func parseInstalledVersions(installPath string) (semver.Collection, error) {
	var parsedVersions []*semver.Version

	files, err := os.ReadDir(installPath)
	if err != nil {
		log.Printf("Failed to find directory %s", err.Error())
		return parsedVersions, errors.Wrapf(err, "Failed to find directory %s", installPath)
	}

	for _, f := range files {
		if !f.IsDir() {
			log.Printf("Unexpected file detected: %s", f.Name())
			continue
		}

		parsedVersion, err := semver.NewVersion(f.Name())
		if err != nil {
			return nil, fmt.Errorf("Failed to parse version: %s", f.Name())
		}
		parsedVersions = append(parsedVersions, parsedVersion)
		log.Printf("Detected folder version: %s\n", f.Name())
	}
	return parsedVersions, nil
}

func downloadVersion(installPath string, version string, versionMeta *catalog.Version) error {
	versionInstallPath := filepath.Join(installPath, version)
	stat, err := os.Stat(versionInstallPath)
	if err == nil {
		if !stat.IsDir() {
			err = os.Remove(versionInstallPath)
			if err != nil {
				return err
			}
		}
	} else {
		if err != nil && os.IsNotExist(err) {
			err := os.Mkdir(versionInstallPath, os.ModePerm)
			if err != nil {
				log.Printf("Make dirs: %s\n", err.Error())
				return err
			}
		} else {
			log.Printf("Failed installation path: %s\n", err.Error())
			return err
		}
	}

	finalPath := filepath.Join(versionInstallPath, "agent")
	present, err := FileExists(finalPath)
	if err != nil {
		return errors.Wrapf(
			err,
			"Failed to detect installed status at %s: %s",
			finalPath,
			err.Error(),
		)
	}
	if present {
		log.Printf("Already installed and unpacked")
		return nil
	}

	log.Printf("Installation start")

	// check if already unpacked
	unpackedInstallPath := filepath.Join(versionInstallPath, "unpack")
	log.Printf("unpacked install path %s", unpackedInstallPath)
	present, err = FileExists(unpackedInstallPath)
	if err != nil {
		return errors.Wrapf(err, "Failed to detect unpack status")
	}
	if present {
		log.Printf("Unpack folder present, clearing out")
		err = os.RemoveAll(unpackedInstallPath)
		if err != nil {
			return errors.Wrapf(err, "Failed to remove current unpack folder")
		}
	} else {
		log.Printf("No unpack folder already present")
	}

	// if not unpacked then check if downloaded and in case download
	// TODO hack for apt
	packageName := fmt.Sprintf("datadog-agent_1:%s_amd64.deb", versionMeta.Source)
	packageNameFile := fmt.Sprintf("datadog-agent_1%%3a%s_amd64.deb", versionMeta.Source)
	log.Println("Trying to download", packageName, "to", packageNameFile)

	// check if a file existing before is complete if exists
	fileReady := false
	downloadedFilePath := filepath.Join(versionInstallPath, packageNameFile)
	present, err = FileExists(downloadedFilePath)
	if present {
		f, err := os.Open(downloadedFilePath)

		log.Printf("File found at %s\n", downloadedFilePath)
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return errors.Wrap(err, "Failed to hash downloaded package")
		}
		hash := fmt.Sprintf("%x", h.Sum(nil))
		if versionMeta.Hash != hash {
			log.Println("Invalid hash. Expected", versionMeta.Hash, "but found", hash)
			err = os.Remove(downloadedFilePath)
			if err != nil {
				return errors.Wrapf(err, "Failed to delete package %s", downloadedFilePath)
			}
		} else {
			log.Println("Hash match! No need to re-download")
			fileReady = true
		}
	}
	if !fileReady {
		log.Println("Download file", downloadedFilePath)
		cmd := exec.Command("apt-get", "download", fmt.Sprintf("datadog-agent=1:%s", version))
		cmd.Dir = versionInstallPath
		err = cmd.Run()
		if err != nil {
			return err
		}
	}

	// now that we know the file is present and it's downloaded we can unpack
	// it again and try to move it
	// unpack in temporary folder to ensure we atomically consider it unpacked

	err = os.Mkdir(unpackedInstallPath, os.ModeDir)
	if err != nil {
		log.Printf("Failed to create unpack folder %s", unpackedInstallPath)
		return err
	}

	cmd := exec.Command("dpkg", "-x", packageNameFile, unpackedInstallPath)
	cmd.Dir = versionInstallPath
	err = cmd.Run()
	if err != nil {
		return errors.Wrap(err, "Failed to unpack")
	}
	// finalize the unpack as installed
	return os.Rename(unpackedInstallPath, finalPath)
}

func (u *Updater) InstallVersion(
	ctx context.Context,
	version string,
	versionMeta *catalog.Version,
) error {
	err := downloadVersion(u.binariesPath, version, versionMeta)
	return err
}

func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
