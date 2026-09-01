// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AdoptExperiment sets the experiment link to a version directory that is already in the
// repository, without moving anything into it.
//
// It exists for macOS, where the payload does not arrive as a directory the installer can move.
// The system installer writes it, and writes it directly into the repository -- the destination is
// baked into the .pkg at build time, because installer(8)'s -target names a volume and not a
// directory. By the time the installer process gets control the version directory is already
// there, which SetExperiment reads as an error ("target package already exists") and which its
// leading cleanup would delete before the link could name it.
//
// The ordering here is the whole point and is the reverse of SetExperiment's: the link is set
// first and only then is the repository cleaned up. Cleaning up first would remove the very
// payload being adopted, because a version no link names is by definition garbage.
func (r *Repository) AdoptExperiment(ctx context.Context, name string) error {
	repository, err := readRepository(r.rootPath, r.preRemoveHooks)
	if err != nil {
		return err
	}
	if name == "" || name == stableVersionLink || name == experimentVersionLink {
		return errors.New("invalid package name")
	}
	if !repository.stable.Exists() {
		return errors.New("stable link does not exist, invalid state")
	}
	if !repository.experiment.Exists() {
		return errors.New("experiment link does not exist, invalid state")
	}
	if filepath.Base(*repository.stable.packagePath) == name {
		return errors.New("cannot set new experiment to the same version as stable")
	}

	packagePath := filepath.Join(r.rootPath, name)
	stat, err := os.Stat(packagePath)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot adopt %s: it is not in the repository", packagePath)
	}
	if err != nil {
		return fmt.Errorf("could not stat %s: %w", packagePath, err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("cannot adopt %s: it is not a directory", packagePath)
	}

	if err := repository.experiment.Set(packagePath); err != nil {
		return fmt.Errorf("could not set experiment: %w", err)
	}
	// Only now: with the link in place the adopted version is named, so cleanup will keep it
	// while reclaiming whatever a previous experiment left behind.
	if err := repository.cleanup(ctx); err != nil {
		return fmt.Errorf("could not cleanup repository: %w", err)
	}
	return nil
}

// HasVersion reports whether a version directory is present in the repository.
//
// It says nothing about whether the payload in it is usable; that question belongs to whoever
// wrote the payload and knows what a complete one looks like.
func (r *Repository) HasVersion(name string) (bool, error) {
	if name == "" || name == stableVersionLink || name == experimentVersionLink {
		return false, errors.New("invalid package name")
	}
	stat, err := os.Stat(filepath.Join(r.rootPath, name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not stat package: %w", err)
	}
	return stat.IsDir(), nil
}
