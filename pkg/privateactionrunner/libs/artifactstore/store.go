// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package artifactstore atomically publishes immutable artifacts in a filesystem cache.

package artifactstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

const (
	artifactsDirectoryName  = "artifacts"
	locksDirectoryName      = "locks"
	stagingDirectoryName    = "staging"
	stagingDirectoryPrefix  = "artifact-"
	lockFileSuffix          = ".lock"
	privateDirectoryMode    = 0o700
	defaultLockPollInterval = 100 * time.Millisecond
)

// Key identifies an artifact variant, such as package/sha256-digest/materialization.
type Key struct {
	Namespace string
	ID        string
	Variant   string
}

// Artifact identifies an artifact directory that is ready for use.
type Artifact struct {
	Directory string
}

// PopulateFunc writes a complete artifact into stagingDirectory.
type PopulateFunc func(ctx context.Context, stagingDirectory string) error

// ValidateFunc verifies an artifact
type ValidateFunc func(ctx context.Context, artifactDirectory string) error

// Store coordinates concurrent access to artifacts
type Store struct {
	root             string
	lockPollInterval time.Duration
}

type storePaths struct {
	artifactDirectory      string
	lockFile               string
	stagingParentDirectory string
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("artifact store root is required")
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("artifact store root %q must be absolute", root)
	}

	cleanRoot := filepath.Clean(root)
	if cleanRoot == filepath.VolumeName(cleanRoot)+string(filepath.Separator) {
		return nil, fmt.Errorf("artifact store root %q cannot be a filesystem root", root)
	}

	return &Store{
		root:             cleanRoot,
		lockPollInterval: defaultLockPollInterval,
	}, nil
}

// Ensure returns a validated artifact entry or populates and atomically publishes one.
func (s *Store) Ensure(ctx context.Context, key Key, populate PopulateFunc, validate ValidateFunc) (artifact Artifact, returnErr error) {
	if err := validateEnsureRequest(ctx, key, populate, validate); err != nil {
		return Artifact{}, err
	}
	if err := createPrivateDirectory(s.root); err != nil {
		return Artifact{}, fmt.Errorf("could not create artifact store root: %w", err)
	}

	paths := s.paths(key)
	// Return immediately when a valid artifact is already cached.
	artifactUsable, err := isArtifactReady(ctx, paths.artifactDirectory, validate)
	if err != nil {
		return Artifact{}, fmt.Errorf("could not inspect artifact %s: %w", key, err)
	}
	if artifactUsable {
		return Artifact{Directory: paths.artifactDirectory}, nil
	}

	if err := createPrivateDirectory(filepath.Dir(paths.lockFile)); err != nil {
		return Artifact{}, fmt.Errorf("could not create artifact lock directory: %w", err)
	}

	// Wait for another publisher to finish, or acquire the artifact lock.
	fileLock := flock.New(paths.lockFile)
	artifactAvailable, err := waitForArtifactOrAcquireLock(ctx, fileLock, s.lockPollInterval, paths.artifactDirectory, validate)
	if err != nil {
		return Artifact{}, fmt.Errorf("could not acquire lock for artifact %s: %w", key, err)
	}
	if artifactAvailable {
		return Artifact{Directory: paths.artifactDirectory}, nil
	}
	defer func() {
		if err := fileLock.Unlock(); err != nil {
			artifact = Artifact{}
			returnErr = errors.Join(returnErr, fmt.Errorf("could not release lock for artifact %s: %w", key, err))
		}
	}()

	// Recheck after locking in case another process published first.
	artifactUsable, err = isArtifactReady(ctx, paths.artifactDirectory, validate)
	if err != nil {
		return Artifact{}, fmt.Errorf("could not inspect artifact %s after acquiring its lock: %w", key, err)
	}
	if artifactUsable {
		return Artifact{Directory: paths.artifactDirectory}, nil
	}

	return populateAndPublish(ctx, key, paths, populate, validate)
}

func validateEnsureRequest(ctx context.Context, key Key, populate PopulateFunc, validate ValidateFunc) error {
	if populate == nil {
		return errors.New("artifact populate function is required")
	}
	if validate == nil {
		return errors.New("artifact validate function is required")
	}
	if err := validateKey(key); err != nil {
		return err
	}
	return ctx.Err()
}

func (s *Store) paths(key Key) storePaths {
	return storePaths{
		artifactDirectory: filepath.Join(
			s.root,
			artifactsDirectoryName,
			key.Namespace,
			key.ID,
			key.Variant,
		),
		lockFile: filepath.Join(
			s.root,
			locksDirectoryName,
			key.Namespace,
			key.ID,
			key.Variant+lockFileSuffix,
		),
		stagingParentDirectory: filepath.Join(
			s.root,
			stagingDirectoryName,
			key.Namespace,
			key.ID,
			key.Variant,
		),
	}
}

func populateAndPublish(ctx context.Context, key Key, paths storePaths, populate PopulateFunc, validate ValidateFunc) (Artifact, error) {
	// Remove an existing entry that failed validation.
	if err := removeCacheTree(paths.artifactDirectory); err != nil {
		return Artifact{}, fmt.Errorf("could not remove unusable artifact %s: %w", key, err)
	}

	// Populate and validate in a private staging directory.
	if err := prepareStagingParent(paths); err != nil {
		return Artifact{}, fmt.Errorf("could not prepare staging for artifact %s: %w", key, err)
	}
	defer func() {
		_ = removeCacheTree(paths.stagingParentDirectory)
	}()

	stagingRoot, err := os.MkdirTemp(paths.stagingParentDirectory, stagingDirectoryPrefix)
	if err != nil {
		return Artifact{}, fmt.Errorf("could not create staging directory for artifact %s: %w", key, err)
	}

	if err := populate(ctx, stagingRoot); err != nil {
		return Artifact{}, fmt.Errorf("could not populate artifact %s: %w", key, err)
	}
	if err := validate(ctx, stagingRoot); err != nil {
		return Artifact{}, fmt.Errorf("could not validate artifact %s: %w", key, err)
	}
	if err := ctx.Err(); err != nil {
		return Artifact{}, err
	}
	rootMode, err := makeContentsReadOnly(stagingRoot)
	if err != nil {
		return Artifact{}, fmt.Errorf("could not make artifact %s read-only: %w", key, err)
	}

	// Atomically publish the complete artifact.
	if err := createPrivateDirectory(filepath.Dir(paths.artifactDirectory)); err != nil {
		return Artifact{}, fmt.Errorf("could not create artifact cache directory: %w", err)
	}
	if err := os.Rename(stagingRoot, paths.artifactDirectory); err != nil {
		return Artifact{}, fmt.Errorf("could not publish artifact %s: %w", key, err)
	}
	if err := os.Chmod(paths.artifactDirectory, rootMode&^0o222); err != nil {
		return Artifact{}, fmt.Errorf("could not make published artifact %s read-only: %w", key, err)
	}

	return Artifact{Directory: paths.artifactDirectory}, nil
}

func validateKey(key Key) error {
	for _, component := range []struct {
		name  string
		value string
	}{
		{name: "namespace", value: key.Namespace},
		{name: "id", value: key.ID},
		{name: "variant", value: key.Variant},
	} {
		if !isPathComponent(component.value) {
			return fmt.Errorf("artifact key %s %q must be a single, non-empty path component", component.name, component.value)
		}
	}
	return nil
}

func isPathComponent(value string) bool {
	return value != "" && value != "." && filepath.IsLocal(value) && !strings.ContainsAny(value, `/\`)
}

func isArtifactUsable(ctx context.Context, directory string, validate ValidateFunc) (bool, error) {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	if err := validate(ctx, directory); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return false, contextErr
		}
		return false, nil
	}
	return true, nil
}

// A writable root marks an incomplete publication; readers wait for its lock.
func isArtifactReady(ctx context.Context, directory string, validate ValidateFunc) (bool, error) {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode().Perm()&0o222 != 0 {
		return false, nil
	}
	return isArtifactUsable(ctx, directory, validate)
}

// makeContentsReadOnly preserves read and execute permissions, and never follows links.
// The root remains writable until after publication so it can be renamed.
func makeContentsReadOnly(root string) (os.FileMode, error) {
	var rootMode os.FileMode
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root && !entry.IsDir() {
			return fmt.Errorf("artifact root is not a directory: %s", root)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("unexpected file type in artifact: %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		if path == root {
			rootMode = mode
			return nil
		}
		if mode.Perm()&0o222 != 0 {
			return os.Chmod(path, mode&^0o222)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return rootMode, nil
}

// removeCacheTree restores owner access so a published package can be removed
// on validation failure, or staging can be cleaned after a crash.
func removeCacheTree(root string) error {
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode() | 0o200
		if entry.IsDir() {
			mode |= 0o500
		}
		return os.Chmod(path, mode)
	})
	if err != nil {
		return err
	}
	return os.RemoveAll(root)
}

func createPrivateDirectory(path string) error {
	if err := os.MkdirAll(path, privateDirectoryMode); err != nil {
		return err
	}
	return os.Chmod(path, privateDirectoryMode)
}

func waitForArtifactOrAcquireLock(
	ctx context.Context,
	fileLock *flock.Flock,
	pollInterval time.Duration,
	artifactDirectory string,
	validate ValidateFunc,
) (artifactAvailable bool, err error) {
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}

		artifactUsable, err := isArtifactReady(ctx, artifactDirectory, validate)
		if err != nil {
			return false, err
		}
		if artifactUsable {
			return true, nil
		}

		locked, err := fileLock.TryLock()
		if err != nil {
			return false, err
		}
		if locked {
			return false, nil
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return false, ctx.Err()
		case <-timer.C:
		}
	}
}

func prepareStagingParent(paths storePaths) error {
	if err := removeCacheTree(paths.stagingParentDirectory); err != nil {
		return err
	}
	return createPrivateDirectory(paths.stagingParentDirectory)
}

func (k Key) String() string {
	return k.Namespace + ":" + k.ID + "/" + k.Variant
}
