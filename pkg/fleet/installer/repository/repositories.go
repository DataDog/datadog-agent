// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

const (
	tempDirPrefix = "tmp-i-"
)

var (
	fsDisk = filesystem.NewDisk()
)

// Repositories manages multiple repositories.
type Repositories struct {
	rootPath  string
	locksPath string
}

// NewRepositories returns a new Repositories.
func NewRepositories(rootPath, locksPath string) *Repositories {
	return &Repositories{
		rootPath:  rootPath,
		locksPath: locksPath,
	}
}

func (r *Repositories) newRepository(pkg string) *Repository {
	return &Repository{
		rootPath:  filepath.Join(r.rootPath, pkg),
		locksPath: filepath.Join(r.locksPath, pkg),
	}
}

func (r *Repositories) loadRepositories() (map[string]*Repository, error) {
	repositories := make(map[string]*Repository)
	dir, err := os.ReadDir(r.rootPath)
	if err != nil {
		return nil, fmt.Errorf("could not open root directory: %w", err)
	}
	for _, d := range dir {
		if !d.IsDir() {
			continue
		}
		if strings.HasPrefix(d.Name(), tempDirPrefix) {
			// Temporary dir created by Repositories.MkdirTemp, ignore
			continue
		}
		if d.Name() == "run" {
			// run dir, ignore
			continue
		}
		repo := r.newRepository(d.Name())
		repositories[d.Name()] = repo
	}
	return repositories, nil
}

// RootPath returns the root path of the repositories.
func (r *Repositories) RootPath() string {
	return r.rootPath
}

// Get returns the repository for the given package name.
func (r *Repositories) Get(pkg string) *Repository {
	return r.newRepository(pkg)
}

// Create creates a new repository for the given package name.
func (r *Repositories) Create(pkg string, version string, stableSourcePath string) error {
	repository := r.newRepository(pkg)
	err := repository.Create(version, stableSourcePath)
	if err != nil {
		return fmt.Errorf("could not create repository for package %s: %w", pkg, err)
	}
	return nil
}

// Delete deletes the repository for the given package name.
func (r *Repositories) Delete(_ context.Context, pkg string) error {
	repository := r.newRepository(pkg)
	// TODO: locked packages will still be deleted
	err := os.RemoveAll(repository.rootPath)
	if err != nil {
		return fmt.Errorf("could not delete repository for package %s: %w", pkg, err)
	}
	return nil
}

// GetStates returns the state of all repositories.
func (r *Repositories) GetStates() (map[string]State, error) {
	state := make(map[string]State)
	repositories, err := r.loadRepositories()
	if err != nil {
		return nil, fmt.Errorf("could not load repositories: %w", err)
	}
	for name, repo := range repositories {
		state[name], err = repo.GetState()
		if err != nil {
			return nil, fmt.Errorf("could not get state for repository %s: %w", name, err)
		}
	}
	return state, nil
}

// GetState returns the state of the given package.
func (r *Repositories) GetState(pkg string) (State, error) {
	repo := r.newRepository(pkg)
	return repo.GetState()
}

// Cleanup cleans up the repositories.
func (r *Repositories) Cleanup() error {
	repositories, err := r.loadRepositories()
	if err != nil {
		return fmt.Errorf("could not load repositories: %w", err)
	}
	for _, repo := range repositories {
		err := repo.Cleanup()
		if err != nil {
			return fmt.Errorf("could not clean up repository: %w", err)
		}
	}
	return nil
}

// MkdirTemp creates a temporary directory in the same partition as the root path.
// This ensures that the temporary directory can be moved to the root path without copying.
// The caller is responsible for cleaning up the directory.
func (r *Repositories) MkdirTemp() (string, error) {
	return os.MkdirTemp(r.rootPath, tempDirPrefix+"*")
}

// AvailableDiskSpace returns the available disk space for the repositories.
// This will check the underlying partition of the given path. Note that the path must be an existing dir.
//
// On Unix, it is computed using `statfs` and is the number of free blocks available to an unprivileged used * block size
// See https://man7.org/linux/man-pages/man2/statfs.2.html for more details
// On Windows, it is computed using `GetDiskFreeSpaceExW` and is the number of bytes available
// See https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw for more details
func (r *Repositories) AvailableDiskSpace() (uint64, error) {
	_, err := os.Stat(r.rootPath)
	if err != nil {
		return 0, fmt.Errorf("could not stat root path %s: %w", r.rootPath, err)
	}
	s, err := fsDisk.GetUsage(r.rootPath)
	if err != nil {
		return 0, err
	}
	return s.Available, nil
}
