// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package repository

import (
	"fmt"
	"os"
	"path/filepath"
)

// Repositories manages multiple repositories.
type Repositories struct {
	rootPath     string
	locksPath    string
	repositories map[string]*Repository
}

// NewRepositories returns a new Repositories.
func NewRepositories(rootPath, locksPath string) (*Repositories, error) {
	r := &Repositories{
		rootPath:     rootPath,
		locksPath:    locksPath,
		repositories: make(map[string]*Repository),
	}
	dir, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, fmt.Errorf("could not open root directory: %w", err)
	}
	for _, d := range dir {
		if !d.IsDir() {
			continue
		}
		repo := &Repository{
			rootPath:  filepath.Join(rootPath, d.Name()),
			locksPath: filepath.Join(locksPath, d.Name()),
		}
		r.repositories[d.Name()] = repo
	}
	return r, nil
}

// Get returns the repository for the given package name.
func (r *Repositories) Get(pkg string) (*Repository, error) {
	repo, ok := r.repositories[pkg]
	if !ok {
		return nil, fmt.Errorf("repository for package %s not found", pkg)
	}
	return repo, nil
}

// Create creates a new repository for the given package name.
func (r *Repositories) Create(pkg string, version string, stableSourcePath string) error {
	repository := &Repository{
		rootPath:  filepath.Join(r.rootPath, pkg),
		locksPath: filepath.Join(r.locksPath, pkg),
	}
	err := repository.Create(version, stableSourcePath)
	if err != nil {
		return fmt.Errorf("could not create repository for package %s: %w", pkg, err)
	}
	r.repositories[pkg] = repository
	return nil
}

// GetState returns the state of all repositories.
func (r *Repositories) GetState() (map[string]State, error) {
	state := make(map[string]State)
	var err error
	for name, repo := range r.repositories {
		state[name], err = repo.GetState()
		if err != nil {
			return nil, fmt.Errorf("could not get state for repository %s: %w", name, err)
		}
	}
	return state, nil
}

// GetPackageState returns the state of the given package.
func (r *Repositories) GetPackageState(pkg string) (State, error) {
	if repo, ok := r.repositories[pkg]; ok {
		return repo.GetState()
	}
	return State{}, nil
}

// Cleanup cleans up the repositories.
func (r *Repositories) Cleanup() error {
	for _, repo := range r.repositories {
		err := repo.Cleanup()
		if err != nil {
			return fmt.Errorf("could not clean up repository: %w", err)
		}
	}
	return nil
}
