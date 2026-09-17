// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package artifactstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testKey = Key{Namespace: "sha256", ID: "abc123", Variant: "v1-linux-amd64"}

func TestNew(t *testing.T) {
	t.Run("requires root", func(t *testing.T) {
		store, err := New("")
		require.ErrorContains(t, err, "root is required")
		assert.Nil(t, store)
	})

	t.Run("requires absolute root", func(t *testing.T) {
		store, err := New("relative")
		require.ErrorContains(t, err, "must be absolute")
		assert.Nil(t, store)
	})

	t.Run("rejects filesystem root", func(t *testing.T) {
		store, err := New(filepath.VolumeName(t.TempDir()) + string(filepath.Separator))
		require.ErrorContains(t, err, "cannot be a filesystem root")
		assert.Nil(t, store)
	})
}

func TestEnsureCachesPopulatedArtifact(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "store"))
	require.NoError(t, err)

	var populateCalls atomic.Int32
	populate := func(_ context.Context, directory string) error {
		populateCalls.Add(1)
		return os.WriteFile(filepath.Join(directory, "artifact"), []byte("valid"), 0o600)
	}
	validate := validateTestArtifact("valid")

	first, err := store.Ensure(context.Background(), testKey, populate, validate)
	require.NoError(t, err)
	second, err := store.Ensure(context.Background(), testKey, populate, validate)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, int32(1), populateCalls.Load())
}

func TestEnsureRepairsInvalidArtifact(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "store"))
	require.NoError(t, err)

	var populateCalls atomic.Int32
	populate := func(_ context.Context, directory string) error {
		populateCalls.Add(1)
		return os.WriteFile(filepath.Join(directory, "artifact"), []byte("valid"), 0o600)
	}

	artifact, err := store.Ensure(context.Background(), testKey, populate, validateTestArtifact("valid"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(artifact.Directory, "artifact"), []byte("corrupt"), 0o600))

	repaired, err := store.Ensure(context.Background(), testKey, populate, validateTestArtifact("valid"))
	require.NoError(t, err)
	assert.Equal(t, artifact.Directory, repaired.Directory)
	assert.Equal(t, int32(2), populateCalls.Load())
}

func TestEnsureSerializesConcurrentPopulation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	firstStore, err := New(root)
	require.NoError(t, err)
	secondStore, err := New(root)
	require.NoError(t, err)
	stores := []*Store{firstStore, secondStore}

	var populateCalls atomic.Int32
	populationStarted := make(chan struct{})
	allowPopulation := make(chan struct{})
	populate := func(_ context.Context, directory string) error {
		if populateCalls.Add(1) == 1 {
			close(populationStarted)
		}
		<-allowPopulation
		return os.WriteFile(filepath.Join(directory, "artifact"), []byte("valid"), 0o600)
	}

	const callers = 16
	errorsByCaller := make([]error, callers)
	var callersStarted sync.WaitGroup
	callersStarted.Add(callers)
	var callersDone sync.WaitGroup
	callersDone.Add(callers)
	for index := range callers {
		go func() {
			defer callersDone.Done()
			callersStarted.Done()
			_, errorsByCaller[index] = stores[index%len(stores)].Ensure(context.Background(), testKey, populate, validateTestArtifact("valid"))
		}()
	}
	callersStarted.Wait()
	<-populationStarted
	close(allowPopulation)
	callersDone.Wait()

	for _, err := range errorsByCaller {
		assert.NoError(t, err)
	}
	assert.Equal(t, int32(1), populateCalls.Load())
}

func TestEnsureCleansFailedPopulation(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "store"))
	require.NoError(t, err)

	expectedErr := errors.New("fetch failed")
	_, err = store.Ensure(
		context.Background(),
		testKey,
		func(_ context.Context, directory string) error {
			if err := os.WriteFile(filepath.Join(directory, "partial"), nil, 0o600); err != nil {
				return err
			}
			return expectedErr
		},
		validateTestArtifact("valid"),
	)
	require.ErrorIs(t, err, expectedErr)
	_, err = os.Stat(store.paths(testKey).stagingKeyDirectory)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestEnsureRejectsUnsafeKey(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "store"))
	require.NoError(t, err)

	for _, key := range []Key{
		{Namespace: "../sha256", ID: "id", Variant: "variant"},
		{Namespace: "sha256", ID: "", Variant: "variant"},
		{Namespace: "sha256", ID: "id", Variant: "nested/variant"},
	} {
		_, err := store.Ensure(context.Background(), key, func(context.Context, string) error { return nil }, func(context.Context, string) error { return nil })
		require.ErrorContains(t, err, "single, non-empty path component")
	}
}

func validateTestArtifact(expected string) ValidateFunc {
	return func(_ context.Context, directory string) error {
		contents, err := os.ReadFile(filepath.Join(directory, "artifact"))
		if err != nil {
			return err
		}
		if string(contents) != expected {
			return errors.New("invalid artifact")
		}
		return nil
	}
}
