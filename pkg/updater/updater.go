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

	"github.com/DataDog/datadog-agent/pkg/updater/repository"
)

const (
	// defaultRepositoryPath is the default path to the repository.
	defaultRepositoryPath = "/opt/datadog-packages"
)

// Install installs the default version for the given package.
// It is purposefully not part of the updater to avoid misuse.
func Install(ctx context.Context, orgConfig *OrgConfig, pkg string) error {
	downloader := newDownloader(http.DefaultClient)
	repository := &repository.Repository{RootPath: path.Join(defaultRepositoryPath, pkg)}
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
}

// NewUpdater returns a new Updater.
func NewUpdater(orgConfig *OrgConfig, pkg string) (*Updater, error) {
	u := &Updater{
		pkg:            pkg,
		repositoryPath: defaultRepositoryPath,
		orgConfig:      orgConfig,
		repository:     &repository.Repository{RootPath: path.Join(defaultRepositoryPath, pkg)},
		downloader:     newDownloader(http.DefaultClient),
	}
	status, err := u.repository.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("could not get repository status: %w", err)
	}
	if !status.HasStable() {
		return nil, fmt.Errorf("attempt to create an updater for a package that has not been bootstrapped with a stable version")
	}
	return u, nil
}

// StartExperiment starts an experiment with the given package.
func (u *Updater) StartExperiment(ctx context.Context, version string) error {
	u.m.Lock()
	defer u.m.Unlock()
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
	return nil
}

// PromoteExperiment promotes the experiment to stable.
func (u *Updater) PromoteExperiment() error {
	u.m.Lock()
	defer u.m.Unlock()
	err := u.repository.PromoteExperiment()
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	return nil
}

// StopExperiment stops the experiment.
func (u *Updater) StopExperiment() error {
	u.m.Lock()
	defer u.m.Unlock()
	err := u.repository.DeleteExperiment()
	if err != nil {
		return fmt.Errorf("could not set stable: %w", err)
	}
	return nil
}
