// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package repository contains the packaging logic of the updater.
package repository

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/process"
)

const (
	previousVersionLink   = "previous"
	stableVersionLink     = "stable"
	experimentVersionLink = "experiment"
)

// Repository contains the stable and experimental package of a single artifact managed by the updater.
//
// On disk the repository is structured as follows:
// .
// ├── 7.50.0
// ├── 7.51.0
// ├── stable -> 7.50.0 (symlink)
// └── experiment -> 7.51.0 (symlink)
//
// and the run directory (if any) is structured as follows:
// .
// ├── 7.50.0
// │   └── 1234
// ├── 7.51.0
// │   └── 5678
//
// We voluntarily do not load the state of the repository in memory to avoid any bugs where
// what's on disk and what's in memory are not in sync.
// All the functions of the repository are "atomic" and ensure no invalid state can be reached
// even if an error happens during their execution.
// It is possible to end up with garbage left on disk if an error happens during some operations. This
// is cleaned up during the next operation.
type Repository struct {
	RootPath string
	m        sync.Mutex

	// RunPath must be set when the updater doesn't control the lifecycle of the
	// process that's experimented on (e.g. tracers).
	//
	// Instead, the updater will put in place a GC mechanism to make sure no process uses the package
	// before removing it from the system. This system will be independent from the experiment process.
	RunPath string
}

// State is the state of the repository.
type State struct {
	Stable     string
	Experiment string
}

// HasStable returns true if the repository has a stable package.
func (s *State) HasStable() bool {
	return s.Stable != ""
}

// HasExperiment returns true if the repository has an experiment package.
func (s *State) HasExperiment() bool {
	return s.Experiment != ""
}

// GetState returns the state of the repository.
func (r *Repository) GetState() (*State, error) {
	repository, err := readRepository(r.RootPath, r.RunPath)
	if err != nil {
		return nil, err
	}
	return &State{
		Stable:     repository.stable.Target(),
		Experiment: repository.experiment.Target(),
	}, nil
}

// Create creates a fresh new repository at the given root path
// and moves the given stable source path to the repository as the first stable.
// If a repository already exists at the given path, it is fully removed.
//
// 1. Remove the previous repository if it exists.
// 2. Create the root directory.
// 3. Move the stable source to the repository.
// 4. Create the stable link.
func (r *Repository) Create(name string, stableSourcePath string) error {
	err := os.MkdirAll(r.RootPath, 0755)
	if err != nil {
		return fmt.Errorf("could not create packages root directory: %w", err)
	}

	if r.RunPath != "" {
		err = createRunDirectory(r.RunPath)
		if err != nil {
			return fmt.Errorf("could not create package's run root directory: %w", err)
		}
	}

	repository, err := readRepository(r.RootPath, r.RunPath)
	if err != nil {
		return err
	}

	// Cleanup (not remove) the previous repository
	err = repository.cleanup()
	if err != nil {
		return fmt.Errorf("could not cleanup repository: %w", err)
	}

	err = repository.setStable(name, stableSourcePath)
	if err != nil {
		return fmt.Errorf("could not set first stable: %w", err)
	}
	return nil
}

// SetExperiment moves package files from the given source path to the repository and sets it as the experiment.
//
// 1. Cleanup the repository.
// 2. Move the experiment source to the repository.
// 3. Set the experiment link to the experiment package.
func (r *Repository) SetExperiment(name string, sourcePath string) error {
	r.m.Lock()
	defer r.m.Unlock()

	repository, err := readRepository(r.RootPath, r.RunPath)
	if err != nil {
		return err
	}
	err = repository.cleanup()
	if err != nil {
		return fmt.Errorf("could not cleanup repository: %w", err)
	}
	if !repository.stable.Exists() {
		return fmt.Errorf("stable package does not exist, invalid state")
	}
	err = repository.setExperiment(name, sourcePath)
	if err != nil {
		return fmt.Errorf("could not set experiment: %w", err)
	}
	return nil
}

// PromoteExperiment promotes the experiment to stable.
//
// 1. Cleanup the repository.
// 2. Set the stable link to the experiment package.
// 3. Delete the experiment link.
// 4. Cleanup the repository to remove the previous stable package.
func (r *Repository) PromoteExperiment() error {
	r.m.Lock()
	defer r.m.Unlock()

	repository, err := readRepository(r.RootPath, r.RunPath)
	if err != nil {
		return err
	}
	err = repository.cleanup()
	if err != nil {
		return fmt.Errorf("could not cleanup repository: %w", err)
	}
	if !repository.stable.Exists() {
		return fmt.Errorf("stable package does not exist, invalid state")
	}
	if !repository.experiment.Exists() {
		return fmt.Errorf("experiment package does not exist, invalid state")
	}
	err = repository.stable.Set(*repository.experiment.packagePath)
	if err != nil {
		return fmt.Errorf("could not set stable: %w", err)
	}
	err = repository.experiment.Delete()
	if err != nil {
		return fmt.Errorf("could not delete experiment link: %w", err)
	}
	err = repository.cleanup()
	if err != nil {
		return fmt.Errorf("could not cleanup repository: %w", err)
	}
	return nil
}

// DeleteExperiment deletes the experiment.
//
// 1. Cleanup the repository.
// 2. Delete the experiment link.
// 3. Cleanup the repository to remove the previous experiment package.
func (r *Repository) DeleteExperiment() error {
	r.m.Lock()
	defer r.m.Unlock()

	repository, err := readRepository(r.RootPath, r.RunPath)
	if err != nil {
		return err
	}
	err = repository.cleanup()
	if err != nil {
		return fmt.Errorf("could not cleanup repository: %w", err)
	}
	if !repository.stable.Exists() {
		return fmt.Errorf("stable package does not exist, invalid state")
	}
	if !repository.experiment.Exists() {
		return nil
	}
	err = repository.experiment.Delete()
	if err != nil {
		return fmt.Errorf("could not delete experiment link: %w", err)
	}
	err = repository.cleanup()
	if err != nil {
		return fmt.Errorf("could not cleanup repository: %w", err)
	}
	return nil
}

// Cleanup calls the cleanup function of the repository with a lock
func (r *Repository) Cleanup() error {
	r.m.Lock()
	defer r.m.Unlock()

	repository, err := readRepository(r.RootPath, r.RunPath)
	if err != nil {
		return err
	}
	return repository.cleanup()
}

type repositoryFiles struct {
	rootPath string
	runPath  string

	stable     *link
	experiment *link
}

func readRepository(rootPath string, runPath string) (*repositoryFiles, error) {
	stableLink, err := newLink(filepath.Join(rootPath, stableVersionLink))
	if err != nil {
		return nil, fmt.Errorf("could not load stable link: %w", err)
	}
	experimentLink, err := newLink(filepath.Join(rootPath, experimentVersionLink))
	if err != nil {
		return nil, fmt.Errorf("could not load experiment link: %w", err)
	}

	return &repositoryFiles{
		rootPath:   rootPath,
		runPath:    runPath,
		stable:     stableLink,
		experiment: experimentLink,
	}, nil
}

func (r *repositoryFiles) setExperiment(name string, sourcePath string) error {
	path, err := movePackageFromSource(name, r.rootPath, r.runPath, sourcePath)
	if err != nil {
		return fmt.Errorf("could not move experiment source: %w", err)
	}

	return r.experiment.Set(path)
}

func (r *repositoryFiles) setStable(name string, sourcePath string) error {
	path, err := movePackageFromSource(name, r.rootPath, r.runPath, sourcePath)
	if err != nil {
		return fmt.Errorf("could not move stable source: %w", err)
	}

	return r.stable.Set(path)
}

func movePackageFromSource(packageName string, rootPath string, runPath string, sourcePath string) (string, error) {
	if packageName == "" || packageName == stableVersionLink || packageName == experimentVersionLink {
		return "", fmt.Errorf("invalid package name")
	}
	targetPath := filepath.Join(rootPath, packageName)
	_, err := os.Stat(targetPath)
	if err == nil {
		// Check if we have long running processes using the package
		// If yes, the GC left them untouched and it is fine to redownload
		// them. If not, the GC should have removed the packages so we throw
		// an error
		if runPath != "" {
			targetRunPath := filepath.Join(runPath, packageName)
			inUse, err := packageVersionInUse(targetRunPath)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("could not check if package version is in use: %w", err)
			}
			if inUse {
				return targetPath, nil // Package is in use, we don't reinstall it
			}
		}
		return "", fmt.Errorf("target package already exists")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("could not stat target package: %w", err)
	}
	err = os.Rename(sourcePath, targetPath)
	if err != nil {
		return "", fmt.Errorf("could not move source: %w", err)
	}

	// Create the associated run path if it doesn't exist
	if runPath != "" {
		err = createRunDirectory(filepath.Join(runPath, packageName))
		if err != nil {
			return "", fmt.Errorf("could not create package's run directory: %w", err)
		}
	}

	return targetPath, nil
}

func (r *repositoryFiles) cleanup() error {
	// If we have no run path, we simply delete versions that are not stable or experiment
	if r.runPath == "" {
		versions, err := os.ReadDir(r.rootPath)
		if err != nil {
			return fmt.Errorf("could not read root directory: %w", err)
		}
		for _, version := range versions {
			isLink := version.Name() == stableVersionLink || version.Name() == experimentVersionLink
			isStable := r.stable.Exists() && r.stable.Target() == version.Name()
			isExperiment := r.experiment.Exists() && r.experiment.Target() == version.Name()
			if isLink || isStable || isExperiment {
				continue
			}
			log.Debugf("removing version %s", version.Name())
			err := os.RemoveAll(filepath.Join(r.rootPath, version.Name()))
			if err != nil {
				return fmt.Errorf("could not remove version: %w", err)
			}
		}
		return nil
	}

	// Else, we only delete the version if no processes are using it
	versions, err := os.ReadDir(r.runPath)
	if err != nil {
		return fmt.Errorf("could not read run directory: %w", err)
	}

	// For each version, get the running PIDs. These PIDs are written by the injector.
	// The injector is ran directly with the service as it's a LD_PRELOAD, and has access
	// to the PIDs.
	for _, version := range versions {
		isLink := version.Name() == stableVersionLink || version.Name() == experimentVersionLink
		isStable := r.stable.Exists() && r.stable.Target() == version.Name()
		isExperiment := r.experiment.Exists() && r.experiment.Target() == version.Name()
		if isLink || isStable || isExperiment {
			continue
		}

		if !version.IsDir() {
			continue
		}

		versionInUse, err := packageVersionInUse(filepath.Join(r.runPath, version.Name()))
		if err != nil {
			log.Errorf("could not check if package version is in use: %v", err)
			continue
		}

		// If no PIDs are running, remove the version
		if !versionInUse {
			log.Debugf("no running PIDs for package %s version %s, removing package", r.rootPath, version.Name())
			if err := os.RemoveAll(filepath.Join(r.rootPath, version.Name())); err != nil {
				log.Errorf("could not remove package directory for version %s: %v", version, err)
			}
			if err := os.RemoveAll(filepath.Join(r.runPath, version.Name())); err != nil {
				log.Errorf("could not remove run directory for version %s: %v", version, err)
			}
		}
	}

	return nil
}

// packageVersionInUse checks if the given package version is in use
func packageVersionInUse(runPath string) (bool, error) {
	runningPIDs := 0

	pids, err := os.ReadDir(runPath)
	if err != nil {
		return false, fmt.Errorf("could not read run directory: %w", err)
	}

	// For each PID, check if it's running
	for _, rawPID := range pids {
		pid, err := strconv.ParseInt(rawPID.Name(), 10, 64)
		if err != nil {
			log.Errorf("could not parse PID: %v", err)
			continue
		}
		processExists, err := process.PidExists(int32(pid))
		if err != nil {
			log.Errorf("could not find process with PID %d: %v", pid, err)
		} else if processExists {
			runningPIDs++
		} else {
			log.Debugf("process with PID %d is stopped, removing PID file", pid)
			err = os.RemoveAll(filepath.Join(runPath, rawPID.Name()))
			if err != nil {
				log.Errorf("could not remove PID file: %v", err)
			}
		}
	}

	return runningPIDs > 0, nil
}

// createRunDirectory creates a run directory at the specified path if it doesn't exist.
// Note that this directory is never fully removed as the updater may
// restart before the process is stopped.
// Must be world writeable as we don't control who writes the PIDs
func createRunDirectory(path string) error {
	if fileInfo, err := os.Stat(path); err == nil {
		if fileInfo.Mode().Perm()&0766 != 0766 {
			return fmt.Errorf("run directory exists but is not world writeable, please update the permissions")
		}
	}
	err := os.MkdirAll(path, 0766)
	if err != nil {
		return err
	}
	return nil
}

type link struct {
	linkPath    string
	packagePath *string
}

func newLink(linkPath string) (*link, error) {
	linkExists, err := linkExists(linkPath)
	if err != nil {
		return nil, fmt.Errorf("could check if link exists: %w", err)
	}
	if !linkExists {
		return &link{
			linkPath: linkPath,
		}, nil
	}
	packagePath, err := linkRead(linkPath)
	if err != nil {
		return nil, fmt.Errorf("could not read link: %w", err)
	}
	_, err = os.Stat(packagePath)
	if err != nil {
		return nil, fmt.Errorf("could not read package: %w", err)
	}

	return &link{
		linkPath:    linkPath,
		packagePath: &packagePath,
	}, nil
}

func (l *link) Exists() bool {
	return l.packagePath != nil
}

func (l *link) Target() string {
	if l.Exists() {
		return filepath.Base(*l.packagePath)
	}
	return ""
}

func (l *link) Set(path string) error {
	err := linkSet(l.linkPath, path)
	if err != nil {
		return fmt.Errorf("could not set link: %w", err)
	}
	l.packagePath = &path
	return nil
}

func (l *link) Delete() error {
	err := linkDelete(l.linkPath)
	if err != nil {
		return fmt.Errorf("could not delete link: %w", err)
	}
	l.packagePath = nil
	return nil
}
