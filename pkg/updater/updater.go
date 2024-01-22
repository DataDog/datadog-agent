// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updater implements the updater.
package updater

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/updater/repository"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// defaultRepositoryPath is the default path to the repository.
	defaultRepositoryPath = "/opt/datadog-packages"
	// defaultRunPath is the default path to the repository's run data
	defaultRunPath = "/var/run/datadog-packages"
	// gcInterval is the interval at which the GC will run
	gcInterval = 1 * time.Hour
)

// Install installs the default version for the given package.
// It is purposefully not part of the updater to avoid misuse.
func Install(ctx context.Context, orgConfig *OrgConfig, pkg string, defaultRootPath string, defaultRunPath string, watchProcesses bool) error {
	log.Infof("Updater: Installing default version of package %s", pkg)
	downloader := newDownloader(http.DefaultClient)
	repository := &repository.Repository{RootPath: path.Join(defaultRootPath, pkg)}
	if watchProcesses {
		repository.RunPath = path.Join(defaultRunPath, pkg)
	}
	firstPackage, err := orgConfig.GetDefaultPackage(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not get default package: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = downloader.Download(ctx, firstPackage, tmpDir)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	err = repository.Create(firstPackage.Version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not create repository: %w", err)
	}
	log.Infof("Updater: Successfully installed default version of package %s", pkg)
	return nil
}

// Updater is the updater used to update packages.
type Updater struct {
	m              sync.Mutex
	pkg            string
	repositoryPath string
	orgConfig      *OrgConfig
	repository     *repository.Repository
	downloader     *downloader
	stopChan       chan struct{}
}

// NewUpdater returns a new Updater.
func NewUpdater(orgConfig *OrgConfig, pkg string, defaultRootPath string, defaultRunPath string, watchProcesses bool) (*Updater, error) {
	repository := &repository.Repository{RootPath: path.Join(defaultRootPath, pkg)}
	if watchProcesses {
		repository.RunPath = path.Join(defaultRunPath, pkg)
	}
	state, err := repository.GetState()
	if err != nil {
		return nil, fmt.Errorf("could not get repository state: %w", err)
	}
	if !state.HasStable() {
		return nil, fmt.Errorf("attempt to create an updater for a package that has not been bootstrapped with a stable version")
	}
	return &Updater{
		pkg:            pkg,
		repositoryPath: defaultRepositoryPath,
		orgConfig:      orgConfig,
		repository:     repository,
		downloader:     newDownloader(http.DefaultClient),
		stopChan:       make(chan struct{}),
	}, nil
}

func (u *Updater) StartGC() {
	go func() {
		for {
			select {
			case <-time.After(gcInterval):
				err := u.repository.Cleanup()
				if err != nil {
					log.Errorf("updater: could not run GC: %v", err)
				}
			case <-u.stopChan:
				return
			}
		}
	}()
}
func (u *Updater) StopGC() {
	u.stopChan <- struct{}{}
}

// StartExperiment starts an experiment with the given package.
func (u *Updater) StartExperiment(ctx context.Context, version string) error {
	u.m.Lock()
	defer u.m.Unlock()
	log.Infof("Updater: Starting experiment for package %s version %s", u.pkg, version)
	experimentPackage, err := u.orgConfig.GetPackage(ctx, u.pkg, version)
	if err != nil {
		return fmt.Errorf("could not get package: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	err = u.downloader.Download(ctx, experimentPackage, tmpDir)
	if err != nil {
		return fmt.Errorf("could not download package: %w", err)
	}
	err = u.repository.SetExperiment(experimentPackage.Version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not set experiment: %w", err)
	}
	log.Infof("Updater: Successfully started experiment for package %s version %s", u.pkg, version)
	return nil
}

// PromoteExperiment promotes the experiment to stable.
func (u *Updater) PromoteExperiment() error {
	u.m.Lock()
	defer u.m.Unlock()
	log.Infof("Updater: Promoting experiment for package %s", u.pkg)
	err := u.repository.PromoteExperiment()
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	log.Infof("Updater: Successfully promoted experiment for package %s", u.pkg)
	return nil
}

// StopExperiment stops the experiment.
func (u *Updater) StopExperiment() error {
	u.m.Lock()
	defer u.m.Unlock()
	log.Infof("Updater: Stopping experiment for package %s", u.pkg)
	err := u.repository.DeleteExperiment()
	if err != nil {
		return fmt.Errorf("could not set stable: %w", err)
	}
	log.Infof("Updater: Successfully stopped experiment for package %s", u.pkg)
	return nil
}
