// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package repository contains the packaging logic of the updater.
package repository

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/gopsutil/process"

	"github.com/DataDog/datadog-agent/pkg/updater/service"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	previousVersionLink   = "previous"
	stableVersionLink     = "stable"
	experimentVersionLink = "experiment"
)

var (
	errRepositoryNotCreated = errors.New("repository not created")
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
// and the locks directory (if any) is structured as follows:
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
	rootPath string

	// locksPath is the path to the locks directory
	// containing the PIDs of the processes using the packages.
	locksPath string
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

// StableFS returns the stable package fs.
func (r *Repository) StableFS() fs.FS {
	return os.DirFS(filepath.Join(r.rootPath, stableVersionLink))
}

// ExperimentFS returns the experiment package fs.
func (r *Repository) ExperimentFS() fs.FS {
	return os.DirFS(filepath.Join(r.rootPath, experimentVersionLink))
}

// GetState returns the state of the repository.
func (r *Repository) GetState() (State, error) {
	repository, err := readRepository(r.rootPath, r.locksPath)
	if errors.Is(err, errRepositoryNotCreated) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	return State{
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
	err := os.MkdirAll(r.rootPath, 0755)
	if err != nil {
		return fmt.Errorf("could not create packages root directory: %w", err)
	}

	repository, err := readRepository(r.rootPath, r.locksPath)
	if err != nil {
		return err
	}

	// Remove symlinks as we are bootstrapping
	if repository.experiment.Exists() {
		err = repository.experiment.Delete()
		if err != nil {
			return fmt.Errorf("could not delete experiment link: %w", err)
		}
	}
	if repository.stable.Exists() {
		err = repository.stable.Delete()
		if err != nil {
			return fmt.Errorf("could not delete stable link: %w", err)
		}
	}

	// Remove left-over locks paths
	packageLocksPaths, err := os.ReadDir(r.locksPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not read locks directory: %w", err)
	}
	for _, pkg := range packageLocksPaths {
		pkgRootPath := filepath.Join(r.rootPath, pkg.Name())
		pkgLocksPath := filepath.Join(r.locksPath, pkg.Name())
		if _, err := os.Stat(pkgRootPath); err != nil && errors.Is(err, os.ErrNotExist) {
			err = os.RemoveAll(pkgLocksPath)
			if err != nil {
				log.Errorf("could not remove package %s locks directory, will retry at next startup: %v", pkgLocksPath, err)
			}
		}
	}

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
	repository, err := readRepository(r.rootPath, r.locksPath)
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
	repository, err := readRepository(r.rootPath, r.locksPath)
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

	// Read repository again to re-load the list of locked packages
	repository, err = readRepository(r.rootPath, r.locksPath)
	if err != nil {
		return err
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
	repository, err := readRepository(r.rootPath, r.locksPath)
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

	// Read repository again to re-load the list of locked packages
	repository, err = readRepository(r.rootPath, r.locksPath)
	if err != nil {
		return err
	}
	err = repository.cleanup()
	if err != nil {
		return fmt.Errorf("could not cleanup repository: %w", err)
	}
	return nil
}

// Cleanup calls the cleanup function of the repository
func (r *Repository) Cleanup() error {
	repository, err := readRepository(r.rootPath, r.locksPath)
	if err != nil {
		return err
	}
	return repository.cleanup()
}

type repositoryFiles struct {
	rootPath       string
	locksPath      string
	lockedPackages map[string]bool

	stable     *link
	experiment *link
}

func readRepository(rootPath string, locksPath string) (*repositoryFiles, error) {
	stableLink, err := newLink(filepath.Join(rootPath, stableVersionLink))
	if err != nil {
		return nil, fmt.Errorf("could not load stable link: %w", err)
	}
	experimentLink, err := newLink(filepath.Join(rootPath, experimentVersionLink))
	if err != nil {
		return nil, fmt.Errorf("could not load experiment link: %w", err)
	}

	// List locked packages
	packages, err := os.ReadDir(rootPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errRepositoryNotCreated
	}
	if err != nil {
		return nil, fmt.Errorf("could not read root directory: %w", err)
	}
	lockedPackages := map[string]bool{}
	for _, pkg := range packages {
		pkgLocksPath := filepath.Join(locksPath, pkg.Name())
		isLocked, err := packageLocked(pkgLocksPath)
		if err != nil {
			log.Errorf("could not check if package version is in use: %v", err)
			continue
		}
		lockedPackages[pkg.Name()] = isLocked
	}

	return &repositoryFiles{
		rootPath:       rootPath,
		locksPath:      locksPath,
		lockedPackages: lockedPackages,
		stable:         stableLink,
		experiment:     experimentLink,
	}, nil
}

func (r *repositoryFiles) setExperiment(name string, sourcePath string) error {
	path, err := movePackageFromSource(name, r.rootPath, r.lockedPackages, sourcePath)
	if err != nil {
		return fmt.Errorf("could not move experiment source: %w", err)
	}

	return r.experiment.Set(path)
}

func (r *repositoryFiles) setStable(name string, sourcePath string) error {
	path, err := movePackageFromSource(name, r.rootPath, r.lockedPackages, sourcePath)
	if err != nil {
		return fmt.Errorf("could not move stable source: %w", err)
	}

	return r.stable.Set(path)
}

func movePackageFromSource(packageName string, rootPath string, lockedPackages map[string]bool, sourcePath string) (string, error) {
	if packageName == "" || packageName == stableVersionLink || packageName == experimentVersionLink {
		return "", fmt.Errorf("invalid package name")
	}
	targetPath := filepath.Join(rootPath, packageName)
	_, err := os.Stat(targetPath)
	if err == nil {
		// Check if we have long running processes using the package
		// If yes, the GC left the package in place so we don't reinstall it, but
		// we don't throw an error either.
		// If not, the GC should have removed the packages so we error.
		if lockedPackages[packageName] {
			return targetPath, nil
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
	if err := os.Chmod(targetPath, 0755); err != nil {
		return "", fmt.Errorf("could not set permissions on package: %w", err)
	}
	if strings.HasSuffix(rootPath, "datadog-agent") {
		if err := service.ChownDDAgent(targetPath); err != nil {
			return "", err
		}
	}

	return targetPath, nil
}

func (r *repositoryFiles) cleanup() error {
	files, err := os.ReadDir(r.rootPath)
	if err != nil {
		return fmt.Errorf("could not read root directory: %w", err)
	}

	// For each version, get the running PIDs. These PIDs are written by the injector.
	// The injector is ran directly with the service as it's a LD_PRELOAD, and has access
	// to the PIDs.
	for _, file := range files {
		isLink := file.Name() == stableVersionLink || file.Name() == experimentVersionLink
		isStable := r.stable.Exists() && r.stable.Target() == file.Name()
		isExperiment := r.experiment.Exists() && r.experiment.Target() == file.Name()
		if isLink || isStable || isExperiment {
			continue
		}

		if !r.lockedPackages[file.Name()] {
			// Package isn't locked, remove it
			pkgRepositoryPath := filepath.Join(r.rootPath, file.Name())
			pkgLocksPath := filepath.Join(r.locksPath, file.Name())
			log.Debugf("package %s isn't locked, removing it", pkgRepositoryPath)
			if err := service.RemoveAll(pkgRepositoryPath); err != nil {
				log.Errorf("could not remove package %s directory, will retry: %v", pkgRepositoryPath, err)
			}
			if err := os.RemoveAll(pkgLocksPath); err != nil {
				log.Errorf("could not remove package %s locks directory, will retry: %v", pkgLocksPath, err)
			}
		}
	}

	return nil
}

// packageLocked checks if the given package version is in use
// by checking if there are PIDs corresponding to running processes
// in the locks directory.
func packageLocked(packagePIDsPath string) (bool, error) {
	pids, err := os.ReadDir(packagePIDsPath)
	if errors.Is(err, os.ErrNotExist) {
		log.Debugf("package locks directory does not exist, no running PIDs")
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not read locks directory: %w", err)
	}

	// For each PID, check if it's running
	for _, rawPID := range pids {
		pid, err := strconv.ParseInt(rawPID.Name(), 10, 64)
		if err != nil {
			log.Errorf("could not parse PID: %v", err)
			continue
		}

		processUsed, err := process.PidExists(int32(pid))
		if err != nil {
			log.Errorf("could not find process with PID %d: %v", pid, err)
			continue
		}

		if processUsed {
			return true, nil
		}

		// PIDs can be re-used, so if the process isn't running we remove the file
		log.Debugf("process with PID %d is stopped, removing PID file", pid)
		err = os.Remove(filepath.Join(packagePIDsPath, fmt.Sprint(pid)))
		if err != nil {
			log.Errorf("could not remove PID file: %v", err)
		}
	}

	return false, nil
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
