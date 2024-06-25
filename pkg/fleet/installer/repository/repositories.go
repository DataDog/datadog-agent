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
		if strings.HasPrefix(d.Name(), "tmp-install") {
			// Temporary extraction dir, ignore
			continue
		}
		repo := r.newRepository(d.Name())
		repositories[d.Name()] = repo
	}
	return repositories, nil
}

// Get returns the repository for the given package name.
func (r *Repositories) Get(pkg string) *Repository {
	return r.newRepository(pkg)
}

// Create creates a new repository for the given package name.
func (r *Repositories) Create(ctx context.Context, pkg string, version string, stableSourcePath string) error {
	repository := r.newRepository(pkg)
	err := repository.Create(ctx, version, stableSourcePath)
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

// GetState returns the state of all repositories.
func (r *Repositories) GetState() (map[string]State, error) {
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

// GetPackageState returns the state of the given package.
func (r *Repositories) GetPackageState(pkg string) (State, error) {
	repo := r.newRepository(pkg)
	return repo.GetState()
}

// Cleanup cleans up the repositories.
func (r *Repositories) Cleanup(ctx context.Context) error {
	repositories, err := r.loadRepositories()
	if err != nil {
		return fmt.Errorf("could not load repositories: %w", err)
	}
	for _, repo := range repositories {
		err := repo.Cleanup(ctx)
		if err != nil {
			return fmt.Errorf("could not clean up repository: %w", err)
		}
	}
	return nil
}
