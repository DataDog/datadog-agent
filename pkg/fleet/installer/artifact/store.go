// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

const (
	cacheDirectoryName      = "cache"
	locksDirectoryName      = "locks"
	stagingDirectoryName    = "staging"
	contentDirectoryName    = "content"
	completionMarkerName    = "complete"
	stagingDirectoryPrefix  = "materialize-"
	lockFileSuffix          = ".lock"
	privateDirectoryMode    = 0o700
	privateFileMode         = 0o600
	defaultLockPollInterval = 100 * time.Millisecond
)

// Key identifies one immutable materialization. All inputs that can change the
// bytes or layout on disk must be represented by the key.
type Key struct {
	Algorithm string
	Digest    string
	Variant   string
}

func newKey(descriptor Descriptor, materializerID string) Key {
	variantInput := strings.Join([]string{
		materializerID,
		descriptor.Package,
		descriptor.Version,
		descriptor.Platform.OS,
		descriptor.Platform.Architecture,
		descriptor.Platform.Variant,
	}, "\x00")
	variantDigest := sha256.Sum256([]byte(variantInput))
	return Key{
		Algorithm: descriptor.Digest.Algorithm().String(),
		Digest:    descriptor.Digest.Encoded(),
		Variant:   hex.EncodeToString(variantDigest[:]),
	}
}

// String returns a stable diagnostic representation of the key.
func (k Key) String() string {
	return k.Algorithm + ":" + k.Digest + "/" + k.Variant
}

// StoredArtifact identifies a complete artifact directory managed by Store.
type StoredArtifact struct {
	Directory string
}

// PopulateFunc writes a complete artifact below destination.
type PopulateFunc func(ctx context.Context, destination string) error

// ValidateFunc verifies an artifact without modifying it.
type ValidateFunc func(ctx context.Context, directory string) error

// Store coordinates immutable artifacts on a local filesystem. Store is safe
// for concurrent use by goroutines and processes that use the same root and key
// scheme. The root and its ancestors must not be writable by untrusted users.
type Store struct {
	root             string
	lockPollInterval time.Duration
}

// NewStore creates a Store. Directories are created lazily by Ensure.
func NewStore(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("artifact store root is required")
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("artifact store root %q must be absolute", root)
	}
	root = filepath.Clean(root)
	if root == filepath.VolumeName(root)+string(filepath.Separator) {
		return nil, fmt.Errorf("artifact store root %q cannot be a filesystem root", root)
	}
	return &Store{root: root, lockPollInterval: defaultLockPollInterval}, nil
}

// Ensure returns a valid published artifact or populates and publishes one.
// Invalid cache entries are repaired while holding the per-key lock.
func (s *Store) Ensure(ctx context.Context, key Key, populate PopulateFunc, validate ValidateFunc) (StoredArtifact, error) {
	if s == nil {
		return StoredArtifact{}, errors.New("artifact store is required")
	}
	if ctx == nil {
		return StoredArtifact{}, errors.New("artifact store context is required")
	}
	if populate == nil {
		return StoredArtifact{}, errors.New("artifact populate function is required")
	}
	if validate == nil {
		return StoredArtifact{}, errors.New("artifact validate function is required")
	}
	if err := validateKey(key); err != nil {
		return StoredArtifact{}, err
	}
	if err := ctx.Err(); err != nil {
		return StoredArtifact{}, err
	}
	if err := createPrivateDirectory(s.root); err != nil {
		return StoredArtifact{}, fmt.Errorf("could not create artifact store root: %w", err)
	}

	paths := s.paths(key)
	usable, err := inspectStoredArtifact(ctx, paths.finalDirectory, validate)
	if err != nil {
		return StoredArtifact{}, fmt.Errorf("could not inspect artifact %s: %w", key, err)
	}
	if usable {
		return StoredArtifact{Directory: contentDirectory(paths.finalDirectory)}, nil
	}

	if err := createPrivateDirectory(filepath.Dir(paths.lockFile)); err != nil {
		return StoredArtifact{}, fmt.Errorf("could not create artifact lock directory: %w", err)
	}
	fileLock := flock.New(paths.lockFile)
	locked, available, err := waitForArtifactOrLock(ctx, fileLock, s.lockPollInterval, paths.finalDirectory, validate)
	if err != nil {
		return StoredArtifact{}, fmt.Errorf("could not acquire lock for artifact %s: %w", key, err)
	}
	if available {
		return StoredArtifact{Directory: contentDirectory(paths.finalDirectory)}, nil
	}
	if !locked {
		return StoredArtifact{}, fmt.Errorf("artifact %s is neither available nor locked", key)
	}
	defer fileLock.Unlock() //nolint:errcheck // the artifact result remains valid if lock cleanup fails

	usable, err = inspectStoredArtifact(ctx, paths.finalDirectory, validate)
	if err != nil {
		return StoredArtifact{}, fmt.Errorf("could not inspect artifact %s while locked: %w", key, err)
	}
	if usable {
		return StoredArtifact{Directory: contentDirectory(paths.finalDirectory)}, nil
	}
	if err := removeAll(paths.finalDirectory); err != nil {
		return StoredArtifact{}, fmt.Errorf("could not remove unusable artifact %s: %w", key, err)
	}

	if err := prepareStagingDirectory(paths.stagingKeyDirectory); err != nil {
		return StoredArtifact{}, fmt.Errorf("could not prepare staging for artifact %s: %w", key, err)
	}
	defer removeAll(paths.stagingKeyDirectory) //nolint:errcheck

	stagingRoot, err := os.MkdirTemp(paths.stagingKeyDirectory, stagingDirectoryPrefix)
	if err != nil {
		return StoredArtifact{}, fmt.Errorf("could not create staging directory for artifact %s: %w", key, err)
	}
	stagingContent := contentDirectory(stagingRoot)
	if err := os.Mkdir(stagingContent, privateDirectoryMode); err != nil {
		return StoredArtifact{}, fmt.Errorf("could not create staging content for artifact %s: %w", key, err)
	}
	if err := populate(ctx, stagingContent); err != nil {
		return StoredArtifact{}, fmt.Errorf("could not populate artifact %s: %w", key, err)
	}
	if err := validate(ctx, stagingContent); err != nil {
		return StoredArtifact{}, fmt.Errorf("could not validate artifact %s: %w", key, err)
	}
	if err := ctx.Err(); err != nil {
		return StoredArtifact{}, err
	}
	if err := createCompletionMarker(stagingRoot); err != nil {
		return StoredArtifact{}, fmt.Errorf("could not mark artifact %s complete: %w", key, err)
	}
	if err := createPrivateDirectory(filepath.Dir(paths.finalDirectory)); err != nil {
		return StoredArtifact{}, fmt.Errorf("could not create artifact cache directory: %w", err)
	}
	if err := os.Rename(stagingRoot, paths.finalDirectory); err != nil {
		return StoredArtifact{}, fmt.Errorf("could not publish artifact %s: %w", key, err)
	}
	return StoredArtifact{Directory: contentDirectory(paths.finalDirectory)}, nil
}

type storePaths struct {
	finalDirectory      string
	lockFile            string
	stagingKeyDirectory string
}

func (s *Store) paths(key Key) storePaths {
	return storePaths{
		finalDirectory:      filepath.Join(s.root, cacheDirectoryName, key.Algorithm, key.Digest, key.Variant),
		lockFile:            filepath.Join(s.root, locksDirectoryName, key.Algorithm, key.Digest, key.Variant+lockFileSuffix),
		stagingKeyDirectory: filepath.Join(s.root, stagingDirectoryName, key.Algorithm, key.Digest, key.Variant),
	}
}

func validateKey(key Key) error {
	for _, component := range []struct{ name, value string }{
		{name: "algorithm", value: key.Algorithm},
		{name: "digest", value: key.Digest},
		{name: "variant", value: key.Variant},
	} {
		if !isPathComponent(component.value) {
			return fmt.Errorf("artifact key %s %q must be a single non-empty path component", component.name, component.value)
		}
	}
	return nil
}

func isPathComponent(value string) bool {
	return value != "" && value != "." && filepath.IsLocal(value) && !strings.ContainsAny(value, `/\`)
}

func inspectStoredArtifact(ctx context.Context, directory string, validate ValidateFunc) (bool, error) {
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
	markerInfo, err := os.Lstat(filepath.Join(directory, completionMarkerName))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !markerInfo.Mode().IsRegular() {
		return false, err
	}
	contentInfo, err := os.Lstat(contentDirectory(directory))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !contentInfo.IsDir() {
		return false, err
	}
	if err := validate(ctx, contentDirectory(directory)); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		return false, nil
	}
	return true, nil
}

func contentDirectory(root string) string { return filepath.Join(root, contentDirectoryName) }

func createPrivateDirectory(directory string) error {
	if err := os.MkdirAll(directory, privateDirectoryMode); err != nil {
		return err
	}
	return os.Chmod(directory, privateDirectoryMode)
}

func waitForArtifactOrLock(ctx context.Context, lock *flock.Flock, interval time.Duration, directory string, validate ValidateFunc) (locked, available bool, err error) {
	for {
		if err := ctx.Err(); err != nil {
			return false, false, err
		}
		usable, err := inspectStoredArtifact(ctx, directory, validate)
		if err != nil {
			return false, false, err
		}
		if usable {
			return false, true, nil
		}
		locked, err := lock.TryLock()
		if err != nil {
			return false, false, err
		}
		if locked {
			return true, false, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return false, false, ctx.Err()
		case <-timer.C:
		}
	}
}

func prepareStagingDirectory(directory string) error {
	if err := removeAll(directory); err != nil {
		return err
	}
	return createPrivateDirectory(directory)
}

func removeAll(directory string) error {
	if _, err := os.Lstat(directory); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(path, privateDirectoryMode)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return os.RemoveAll(directory)
}

func createCompletionMarker(directory string) error {
	marker, err := os.OpenFile(filepath.Join(directory, completionMarkerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, privateFileMode)
	if err != nil {
		return err
	}
	return marker.Close()
}
