package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/rc-update-client/pkg/catalog"
	"github.com/pkg/errors"
)

type VersionManager struct {
	updater *Updater
	ctx     context.Context
	cancel  context.CancelFunc
	catalog catalog.Catalog

	runningVersion string

	updateChan <-chan Config
}

func NewVersionManager(
	updater *Updater,
	catalog catalog.Catalog,
	updateChan <-chan Config,
) *VersionManager {
	ctx, cc := context.WithCancel(context.TODO())
	return &VersionManager{
		updater:    updater,
		ctx:        ctx,
		cancel:     cc,
		catalog:    catalog,
		updateChan: updateChan,
	}
}

func (vm *VersionManager) Start() error {
	// TODO it expects to have no agent running, might be better to check & stop

	log.Printf("Version manager starting")
	var versions Config
	select {
	case <-vm.ctx.Done():
		return nil
	case <-time.After(30 * time.Second):
		log.Printf("Timeout waiting for initial version")
		// can't wait longer, going default
	case versions = <-vm.updateChan:
		log.Printf("Received initial version. %+v", versions)
		// got a first update
	}

	err := vm.getVersionAndInstall(versions)
	if err != nil {
		return err
	}

	go vm.updaterLoop()

	return nil
}

func (vm *VersionManager) Stop() error {
	vm.cancel()
	return stopStandardAgent()
}

func (vm *VersionManager) RunVersionChange() error {
	return fmt.Errorf("Not Implemented Yet")
}

func (vm *VersionManager) updaterLoop() {
	for {
		var versions Config
		select {
		case <-vm.ctx.Done():
			return
		case versions = <-vm.updateChan:
		}

		log.Printf("Received a new configuration: %+v", versions)

		err := vm.getVersionAndInstall(versions)
		if err != nil {
			log.Printf("ERROR: get version and install failed: %s", err.Error())
		}
	}
}

func (vm *VersionManager) getVersionAndInstall(versions Config) error {
	version := versions.Agent.Version
	if version == "" {
		version = vm.catalog.GetLatest()
		log.Printf("Using default version: %s", version)
	}

	log.Printf("Running version %s. Configured version %s", vm.runningVersion, version)
	if vm.runningVersion == version {
		return nil
	}

	err := vm.install(version)
	if err != nil {
		return errors.Wrapf(err, "ERROR - couldn't install %s", version)
	}

	// TODO move out of here
	if vm.runningVersion != version {
		stopStandardAgent()
		vm.runningVersion = version
		vm.startStandardAgent()
	}

	// the one to run as a standard state

	return nil
}

// just installs versions. doesn't cleanup, doesn't change links
func (vm *VersionManager) install(version string) error {
	meta, err := vm.catalog.GetVersion(version)
	if err != nil {
		log.Printf("Failed to get base version data: %s\n", version)
	}

	log.Printf("Installing version %s", version)
	err = vm.updater.InstallVersion(vm.ctx, version, meta)
	if err != nil {
		return errors.Wrapf(err, "Failed to install version %s", version)
	}
	log.Println("Installed!", version)

	return err
}

func ensureLink(path string, destination string) error {
	pathExists, err := FileExists(path)
	if err != nil {
		return err
	}
	if pathExists {
		log.Printf("Remove old link")
		err = os.Remove(path)
		if err != nil {
			errors.Wrapf(err, "Failed to remove previous link. %s", path)
		}
	}

	log.Printf("Create link %s -> %s", path, destination)
	err = os.Symlink(destination, path)
	if err != nil {
		return errors.Wrapf(
			err,
			"Failed to create link to default agent: %s -> %s",
			path,
			destination,
		)
	}
	log.Printf("Done")

	return nil
}

func (vm *VersionManager) startStandardAgent() error {
	err := ensureLink(
		"/opt/datadog-agent",
		filepath.Join(agentInstallPath, vm.runningVersion, "agent/opt/datadog-agent/"),
	)
	if err != nil {
		return err
	}

	log.Printf("start agent")
	cmd := exec.Command("systemctl", "start", "datadog-agent")
	err = cmd.Run()
	return err
}

func stopStandardAgent() error {
	log.Printf("stop agent")
	cmd := exec.Command("systemctl", "stop", "datadog-agent")
	err := cmd.Run()
	return err
}

func startExperimentalAgent() error {
	log.Printf("start experimental agent")
	cmd := exec.Command("systemctl", "start", "datadog-agent-experimental")
	err := cmd.Run()
	return err
}

func stopExperimentalAgent() error {
	log.Printf("stop experimental agent")
	cmd := exec.Command("systemctl", "stop", "datadog-agent-experimental")
	err := cmd.Run()
	return err
}
