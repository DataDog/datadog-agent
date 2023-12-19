// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packaging

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
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
// We voluntarily do not load the state of the repository in memory to avoid any bugs where
// what's on disk and what's in memory are not in sync.
// All the functions of the repository are "atomic" and ensure no invalid state can be reached
// even if an error happens during their execution.
// It is possible to end up with garbage left on disk if an error happens during some operations. This
// is cleaned up during the next operation.
type Repository struct {
	RootPath string
}

// CreateRepository creates a fresh new repository at the given root path
// and moves the given stable source path to the repository as the first stable.
// If a repository already exists at the given path, it is fully removed.
//
// 1. Remove the previous repository if it exists.
// 2. Create the root directory.
// 3. Move the stable source to the repository.
// 4. Create the stable link.
func (r *Repository) Create(stableSourcePath string) error {
	err := os.RemoveAll(r.RootPath)
	if err != nil {
		return fmt.Errorf("could not remove previous repository: %w", err)
	}
	err = os.MkdirAll(r.RootPath, 0755)
	if err != nil {
		return fmt.Errorf("could not create packages root directory: %w", err)
	}
	repository, err := openRepository(r.RootPath)
	if err != nil {
		return err
	}
	err = repository.setStable(stableSourcePath)
	if err != nil {
		return fmt.Errorf("could not set first stable: %w", err)
	}
	return nil
}

// SetExperiment moves the given source path to the repository and sets it as the experiment.
//
// 1. Cleanup the repository.
// 2. Move the experiment source to the repository.
// 3. Set the experiment link to the experiment package.
func (r *Repository) SetExperiment(sourcePath string) error {
	repository, err := openRepository(r.RootPath)
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
	err = repository.setExperiment(sourcePath)
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
	repository, err := openRepository(r.RootPath)
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
	repository, err := openRepository(r.RootPath)
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

type repository struct {
	rootPath string

	stable     *link
	experiment *link
}

func openRepository(rootPath string) (*repository, error) {
	stableLink, err := newLink(filepath.Join(rootPath, stableVersionLink))
	if err != nil {
		return nil, fmt.Errorf("could not load stable link: %w", err)
	}
	experimentLink, err := newLink(filepath.Join(rootPath, experimentVersionLink))
	if err != nil {
		return nil, fmt.Errorf("could not load experiment link: %w", err)
	}

	return &repository{
		rootPath:   rootPath,
		stable:     stableLink,
		experiment: experimentLink,
	}, nil
}

func (r *repository) setExperiment(sourcePath string) error {
	path, err := movePackageFromSource(r.rootPath, sourcePath)
	if err != nil {
		return fmt.Errorf("could not move experiment source: %w", err)
	}
	return r.experiment.Set(path)
}

func (r *repository) setStable(sourcePath string) error {
	path, err := movePackageFromSource(r.rootPath, sourcePath)
	if err != nil {
		return fmt.Errorf("could not move stable source: %w", err)
	}
	return r.stable.Set(path)
}

func movePackageFromSource(rootPath string, sourcePath string) (string, error) {
	packageName := filepath.Base(sourcePath)
	if packageName == stableVersionLink || packageName == experimentVersionLink {
		return "", fmt.Errorf("invalid package name")
	}
	targetPath := filepath.Join(rootPath, packageName)
	_, err := os.Stat(targetPath)
	if err == nil {
		return "", fmt.Errorf("target package already exists")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("could not stat target package: %w", err)
	}
	err = os.Rename(sourcePath, targetPath)
	if err != nil {
		return "", fmt.Errorf("could not move source: %w", err)
	}
	return targetPath, nil
}

func (r *repository) cleanup() error {
	files, err := ioutil.ReadDir(r.rootPath)
	if err != nil {
		return fmt.Errorf("could not read root directory: %w", err)
	}
	for _, file := range files {
		isLink := file.Name() == stableVersionLink || file.Name() == experimentVersionLink
		isStable := r.stable.Exists() && r.stable.Target() == file.Name()
		isExperiment := r.experiment.Exists() && r.experiment.Target() == file.Name()
		if isLink || isStable || isExperiment {
			continue
		}
		err := os.RemoveAll(filepath.Join(r.rootPath, file.Name()))
		if err != nil {
			return fmt.Errorf("could not remove file: %w", err)
		}
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
	return filepath.Base(*l.packagePath)
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
