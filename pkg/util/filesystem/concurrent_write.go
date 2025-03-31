// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const lockSuffix = ".lock"
const retryDelay = 500 * time.Millisecond

// ArtifactBuilder is a generic interface for building, serializing, and deserializing artifacts.
// The type parameter T represents the in-memory type of the artifact.
type ArtifactBuilder[T any] interface {
	// Generate creates a new artifact and returns it along with its serialized form.
	Generate() (T, []byte, error)
	// Deserialize converts a serialized artifact into an in-memory representation.
	Deserialize([]byte) (T, error)
}

// FetchArtifact attempts to fetch an artifact from the specified location using the provided factory.
// This function is blocking and will keep retrying until either the artifact is successfully retrieved
// or the provided context is done. If the context is done before the artifact is retrieved, it returns
// an error indicating that the artifact could not be read in the given time.
func FetchArtifact[T any](ctx context.Context, location string, factory ArtifactBuilder[T]) (T, error) {
	var zero T
	for {
		res, err := TryFetchArtifact(location, factory)
		if err == nil {
			return res, nil
		}

		select {
		case <-ctx.Done():
			return zero, fmt.Errorf("unable to read the artifact in the given time")
		case <-time.After(retryDelay):
			// try again
		}
	}
}

// TryFetchArtifact attempts to load an artifact using the provided factory.
// If the artifact does not exist, it return an error.
func TryFetchArtifact[T any](location string, factory ArtifactBuilder[T]) (T, error) {
	var zero T

	// Read the artifact
	content, err := os.ReadFile(location)
	if err != nil {
		return zero, fmt.Errorf("unable to read artifact: %s", err)
	}

	// Try to load artifact
	res, err := factory.Deserialize(content)
	return res, err
}

// FetchOrCreateArtifact attempts to load an artifact using the provided factory.
// If the artifact does not exist, it generates a new one, stores it, and returns it.
//
// The function first tries to load the artifact using the provided location.
// If loading fails, it generates a temporary artifact and attempts to acquire a file lock.
// When the lock is acquired, the function checks if another process has already created the artifact.
// If not, it moves the temporary artifact to its final location.
//
// The function will repeatedly try to acquire the lock until the context is canceled or the lock is acquired.
//
// This function is thread-safe and non-blocking.
func FetchOrCreateArtifact[T any](ctx context.Context, location string, factory ArtifactBuilder[T]) (T, error) {
	var zero T
	var succeed bool

	res, err := TryFetchArtifact(location, factory)
	if err == nil {
		return res, nil
	}

	fileLock := flock.New(location + lockSuffix)
	defer func() {
		log.Debugf("trying to releasing lock for file %v", location)

		// Calling Unlock() even if the lock was not acquired is safe
		// [flock.Unlock()](https://pkg.go.dev/github.com/gofrs/flock#Flock.Unlock) is idempotent
		// Unlock() also close the file descriptor
		err := fileLock.Unlock()
		if err != nil {
			log.Warnf("unable to release lock: %v", err)
		}

		// In a matter of letting the FS cleaned, we should remove the lock file
		// We can consider that if either the artifact have been successfully created or retrieved, the lock file is no longer useful.
		// On UNIX, it is possible to remove file open by another process, but the file will be removed only when the last process close it, so:
		// - process that already opened it will still try to lock it, and when getting the lock, they will successfully load the artifact
		// - process that didn't locked it yet will be able to load the artifact before trying to acquire the lock
		// We filter the error to avoid logging an error if the file does not exist, which would mean that another process already cleaned it
		//
		// On windows, it is not possible to remove a file open by another process, so the remove call will succeed only for the last process that locked it
		if succeed {
			if err = os.Remove(location + lockSuffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
				log.Debugf("unable to remove lock file: %v", err)
			}
		}
	}()

	var lockErr error

	// trying to read artifact or locking file
	for {
		// First check if another process were able to create and save artifact during wait
		res, err := TryFetchArtifact(location, factory)
		if err == nil {
			succeed = true
			return res, nil
		}

		// Trying to acquire lock
		ok, err := fileLock.TryLock()
		if err != nil {
			lockErr = err
			log.Debugf("unable to acquire lock: %v", err)
		}
		if ok {
			break
		}

		select {
		case <-ctx.Done():
			return zero, errors.Join(fmt.Errorf("unable to read the artifact or acquire the lock in the given time"), lockErr)
		case <-time.After(retryDelay):
			// try again
		}
	}

	// Here we acquired the lock
	log.Debugf("lock acquired for file %v", location)

	// First check if another process were able to create and save artifact during lock
	res, err = TryFetchArtifact(location, factory)
	if err == nil {
		succeed = true
		return res, nil
	}

	perms, err := NewPermission()
	if err != nil {
		return zero, log.Errorf("unable to init NewPermission: %v", err)
	}

	// If we are here, it means that the artifact does not exist, and we can expect that this process is the first to lock it
	// and create it (except in case of a previous failure).
	// If the process is run by a high-privileged user (root or Administrator), the lock file will be owned by this user.
	// We must set the permissions to `dd-agent` or an equivalent user to allow other Agent processes to acquire the lock.
	err = perms.RestrictAccessToUser(location + lockSuffix)
	if err != nil {
		return zero, fmt.Errorf("unable to restrict access to user: %v", err)
	}

	createdArtifact, tmpLocation, err := generateTmpArtifact(location, factory, perms)
	if err != nil {
		return zero, fmt.Errorf("unable to generate temporary artifact: %v", err)
	}

	// Move the temporary artifact to its final location, this is an atomic operation
	// and guarantees that the artifact is either fully written or not at all.
	err = os.Rename(tmpLocation, location)
	if err != nil {
		removeErr := os.Remove(tmpLocation)
		if removeErr != nil {
			log.Warnf("unable to remove temporary artifact: %v", removeErr.Error())
		}

		return zero, fmt.Errorf("unable to move temporary artifact to its final location: %v", err)
	}

	log.Debugf("successfully created artifact %v", location)

	succeed = true
	return createdArtifact, nil
}

// tryLockContext tries to acquire a lock on the provided file.
// It copy the behavior of flock.TryLock() but retry if the lock have the wrong permissions.

func generateTmpArtifact[T any](location string, factory ArtifactBuilder[T], perms *Permission) (T, string, error) {
	var zero T

	tmpArtifact, newArtifactContent, err := factory.Generate()
	if err != nil {
		return zero, "", fmt.Errorf("unable to generate new artifact: %v", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(location), "tmp-artifact-")
	if err != nil {
		return zero, "", fmt.Errorf("unable to create temporary artifact: %v", err)
	}
	defer tmpFile.Close()

	tmpLocation := tmpFile.Name()

	_, err = tmpFile.Write(newArtifactContent)
	if err != nil {
		return zero, tmpLocation, fmt.Errorf("unable to store temporary artifact: %v", err)
	}

	//Make sure that data has been written to disk
	if err := tmpFile.Sync(); err != nil {
		return zero, tmpLocation, fmt.Errorf("unable to sync file on disk: %v", err)
	}

	if err := perms.RestrictAccessToUser(tmpLocation); err != nil {
		return zero, tmpLocation, fmt.Errorf("unable to set permission to temporary artifact: %v", err)
	}

	return tmpArtifact, tmpLocation, nil
}
