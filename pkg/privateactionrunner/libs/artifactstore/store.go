// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package artifactstore provides concurrent, atomic publication of immutable
// artifacts in a filesystem cache.
//
// A Store must be rooted in a dedicated directory on a local filesystem that
// provides reliable file locks and atomic renames. The root and its ancestors
// must not be writable by untrusted users: Store intentionally relies on that
// ownership boundary rather than defending against symlink replacement by an
// attacker with write access to the cache hierarchy.
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

// Key identifies one immutable, materialized artifact variant. Namespace is
// typically a digest algorithm, ID is typically a digest, and Variant
// distinguishes materializations such as operating-system and architecture.
// Each field must be a single, non-empty path component. Every input that can
// affect the artifact must be represented in the key, and callers sharing a
// key must use equivalent population and validation semantics.
type Key struct {
	Namespace string
	ID        string
	Variant   string
}

// Artifact identifies an immutable artifact directory that is ready for use.
type Artifact struct {
	Directory string
}

// PopulateFunc writes an artifact in stagingDirectory. It must return nil only
// when it has finished writing the artifact. The Store owns stagingDirectory
// and removes it after PopulateFunc returns.
type PopulateFunc func(ctx context.Context, stagingDirectory string) error

// ValidateFunc verifies an artifact without modifying it. It is called on the
// staging directory before publication and on published cache entries.
// ValidateFunc may run concurrently in multiple processes and should be
// deterministic and safe for concurrent use.
type ValidateFunc func(ctx context.Context, artifactDirectory string) error

// Store coordinates access to immutable artifacts below a filesystem root.
// Store values are safe for concurrent use by goroutines and processes that
// use the same root and key scheme. Published artifacts must not be modified or
// evicted outside Store.
type Store struct {
	root             string
	lockPollInterval time.Duration
}

// New creates an artifact store rooted at root. Directories are created lazily
// by Ensure. The root must be absolute, dedicated to Store, and owned by the
// user running all participating processes. Store restricts it to mode 0700.
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

// Ensure returns a validated, previously published artifact or populates and
// atomically publishes it. Concurrent callers for the same key serialize
// population and repair through a filesystem lock; callers for different keys
// can proceed independently. An invalid cached artifact is replaced while its
// key lock is held.
func (s *Store) Ensure(ctx context.Context, key Key, populate PopulateFunc, validate ValidateFunc) (artifact Artifact, returnErr error) {
	if s == nil {
		return Artifact{}, errors.New("artifact store is required")
	}
	if ctx == nil {
		return Artifact{}, errors.New("artifact store context is required")
	}
	if populate == nil {
		return Artifact{}, errors.New("artifact populate function is required")
	}
	if validate == nil {
		return Artifact{}, errors.New("artifact validate function is required")
	}
	if err := validateKey(key); err != nil {
		return Artifact{}, err
	}
	if err := ctx.Err(); err != nil {
		return Artifact{}, err
	}
	if err := createPrivateDirectory(s.root); err != nil {
		return Artifact{}, fmt.Errorf("could not create artifact store root: %w", err)
	}

	paths := s.paths(key)
	usable, err := inspectArtifact(ctx, paths.finalDirectory, validate)
	if err != nil {
		return Artifact{}, fmt.Errorf("could not inspect artifact %s: %w", key, err)
	}
	if usable {
		return Artifact{Directory: paths.finalDirectory}, nil
	}

	if err := createPrivateDirectory(filepath.Dir(paths.lockFile)); err != nil {
		return Artifact{}, fmt.Errorf("could not create artifact lock directory: %w", err)
	}

	fileLock := flock.New(paths.lockFile)
	available, err := waitForArtifactOrLock(ctx, fileLock, s.lockPollInterval, paths.finalDirectory, validate)
	if err != nil {
		return Artifact{}, fmt.Errorf("could not acquire lock for artifact %s: %w", key, err)
	}
	if available {
		return Artifact{Directory: paths.finalDirectory}, nil
	}
	defer func() {
		if err := fileLock.Unlock(); err != nil {
			// Preserve the usual result contract: an error never accompanies an
			// artifact that callers might accidentally continue using.
			artifact = Artifact{}
			returnErr = errors.Join(returnErr, fmt.Errorf("could not release lock for artifact %s: %w", key, err))
		}
	}()

	// Another process may have published between our last inspection and our
	// successful lock acquisition, so the check while holding the lock is
	// required.
	usable, err = inspectArtifact(ctx, paths.finalDirectory, validate)
	if err != nil {
		return Artifact{}, fmt.Errorf("could not inspect artifact %s after acquiring its lock: %w", key, err)
	}
	if usable {
		return Artifact{Directory: paths.finalDirectory}, nil
	}

	// Removing an unusable entry while holding the per-key lock lets this caller
	// repair the cache while all other callers wait.
	if err := os.RemoveAll(paths.finalDirectory); err != nil {
		return Artifact{}, fmt.Errorf("could not remove unusable artifact %s: %w", key, err)
	}

	if err := prepareStagingDirectory(paths); err != nil {
		return Artifact{}, fmt.Errorf("could not prepare staging for artifact %s: %w", key, err)
	}
	defer func() {
		_ = os.RemoveAll(paths.stagingKeyDirectory)
	}()

	stagingRoot, err := os.MkdirTemp(paths.stagingKeyDirectory, stagingDirectoryPrefix)
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

	if err := createPrivateDirectory(filepath.Dir(paths.finalDirectory)); err != nil {
		return Artifact{}, fmt.Errorf("could not create artifact cache directory: %w", err)
	}
	if err := os.Rename(stagingRoot, paths.finalDirectory); err != nil {
		return Artifact{}, fmt.Errorf("could not publish artifact %s: %w", key, err)
	}

	return Artifact{Directory: paths.finalDirectory}, nil
}

type storePaths struct {
	finalDirectory      string
	lockFile            string
	stagingKeyDirectory string
}

func (s *Store) paths(key Key) storePaths {
	return storePaths{
		finalDirectory: filepath.Join(
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
		stagingKeyDirectory: filepath.Join(
			s.root,
			stagingDirectoryName,
			key.Namespace,
			key.ID,
			key.Variant,
		),
	}
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

func inspectArtifact(ctx context.Context, directory string, validate ValidateFunc) (bool, error) {
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
		// Cancellation is an operation failure, not evidence that the cached
		// artifact is corrupt. In particular, a canceled lock holder must not
		// remove an otherwise valid artifact.
		if contextErr := ctx.Err(); contextErr != nil {
			return false, contextErr
		}
		return false, nil
	}
	return true, nil
}

func createPrivateDirectory(path string) error {
	if err := os.MkdirAll(path, privateDirectoryMode); err != nil {
		return err
	}
	return os.Chmod(path, privateDirectoryMode)
}

func waitForArtifactOrLock(
	ctx context.Context,
	fileLock *flock.Flock,
	pollInterval time.Duration,
	artifactDirectory string,
	validate ValidateFunc,
) (available bool, err error) {
	// A true result means a published artifact is available and no lock is held.
	// A false result with no error means the caller acquired fileLock and must
	// release it.
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}

		// Recheck before every lock attempt. Once another caller publishes a
		// valid immutable artifact, all waiters can use it concurrently instead
		// of acquiring and releasing the lock one at a time.
		usable, err := inspectArtifact(ctx, artifactDirectory, validate)
		if err != nil {
			return false, err
		}
		if usable {
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

func prepareStagingDirectory(paths storePaths) error {
	// The per-key lock guarantees that no active population uses this directory.
	// Removing it cleans staging directories left by a process that exited before
	// its deferred cleanup could run.
	if err := os.RemoveAll(paths.stagingKeyDirectory); err != nil {
		return err
	}
	return createPrivateDirectory(paths.stagingKeyDirectory)
}

func (k Key) String() string {
	return k.Namespace + ":" + k.ID + "/" + k.Variant
}
