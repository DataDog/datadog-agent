// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ArtifactBuilder is a generic interface for building, serializing, and deserializing artifacts.
// The type parameter T represents the in-memory type of the artifact.
type ArtifactBuilder[T any] interface {
	Generate() (T, error)
	Serialize(T) ([]byte, error)
	Deserialize([]byte) (T, error)
}

// FetchArtifact attempts to load an artifact using the provided factory.
// If the artifact does not exist, it return an error.
func FetchArtifact[T any](location string, factory ArtifactBuilder[T]) (T, error) {
	var zero T

	// Read the artifact
	content, err := os.ReadFile(location)
	if err != nil {
		return zero, fmt.Errorf("unable to read artifact: %s", err.Error())
	}

	// Try to load artifact
	res, err := factory.Deserialize(content)
	return res, err
}

// FetchOrCreateArtifact attempts to load an artifact using the provided factory.
// If the artifact does not exist, it generates a new one, stores it, and returns it.
//
// The function first tries to load the artifact using the factory's location.
// If loading fails, it generates a temporary artifact and attempts to acquire a file lock.
// When the lock is acquired, the function checks if another process has already created the artifact.
// If not, it moves the temporary artifact to its final location.
//
// The function will repeatedly try to acquire the lock until the context is canceled or the lock is acquired.
//
// This function is thread-safe and non-blocking.
func FetchOrCreateArtifact[T any](ctx context.Context, location string, factory ArtifactBuilder[T]) (T, error) {
	var zero T

	res, err := FetchArtifact(location, factory)
	if err == nil {
		return res, nil
	}

	// Happy path is to be able to generate and store artifact
	// We prefer generating a temporary artifact and moving it to its final location
	// to avoid having a half written artifact in case of a failure
	createdArtifact, tmpLocation, err := generateTmpArtifact(location, factory)
	if err != nil {
		return zero, fmt.Errorf("unable to generate temporary artifact: %v", err.Error())
	}

	fileLock := flock.New(location + ".lock")
	defer func() {
		log.Debugf("releasing lock for file %v", location)
		err := fileLock.Unlock()
		// We don't want to remove the lock file here, as it may be used by another process
		if err != nil {
			log.Warnf("unable to release lock: %v", err)
		}
	}()

	// trying to lock artifact file
	locked, err := fileLock.TryLockContext(ctx, 100*time.Millisecond)

	if err != nil || !locked {
		if err == nil {
			err = fmt.Errorf("unknown error")
		}
		if !locked {
			err = fmt.Errorf("unable to acquire lock %v, if the error persists, consider removing it manually (err: %v)", location, err.Error())
		}
		return zero, fmt.Errorf("an error happen when trying to acquire lock: %v", err)
	}

	// Here we acquired the lock
	log.Debugf("lock acquired for file %v", location)

	// First check if another process were able to create and save artifact during lock
	res, err = FetchArtifact(location, factory)
	if err == nil {
		// Cleanup the tmp file
		if err := os.Remove(tmpLocation); err != nil {
			log.Warnf("unable to remove temporary artifact: %v", err)
		}

		return res, nil
	}

	// otherwise, we can safely move the temporary artifact to its final location
	err = os.Rename(tmpLocation, location)
	if err != nil {
		return zero, fmt.Errorf("unable to move temporary artifact to its final location: %v", err.Error())
	}

	return createdArtifact, nil
}

func generateTmpArtifact[T any](location string, factory ArtifactBuilder[T]) (T, string, error) {
	var zero T

	tmpArtifact, err := factory.Generate()
	if err != nil {
		return zero, "", fmt.Errorf("unable to generate new artifact: %v", err)
	}

	newArtifactContent, err := factory.Serialize(tmpArtifact)
	if err != nil {
		return zero, "", fmt.Errorf("unable to serialize new artifact: %v", err)
	}

	perms, err := NewPermission()
	if err != nil {
		return zero, "", fmt.Errorf("unable to prepare artifact: %v", err)
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
