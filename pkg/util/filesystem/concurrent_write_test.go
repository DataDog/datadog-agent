// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"context"
	"encoding/hex"
	"os"
	"path"
	"testing"
	"testing/synctest"
	"time"

	"crypto/rand"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

type MockArtiFactory struct {
	t             *testing.T
	data          string
	dataGenerator func() string
	id            int
}

func (m *MockArtiFactory) Generate() (string, []byte, error) {
	data := m.data
	m.t.Logf("artifaction generation starts from %d", m.id)
	if m.dataGenerator != nil {
		data = m.dataGenerator()
	}
	m.t.Logf("artifaction generation ends from %d", m.id)
	return data, []byte(data), nil
}

func (m *MockArtiFactory) Deserialize(data []byte) (string, error) {
	return string(data), nil
}

func newMockArtiFactory(t *testing.T) (string, *MockArtiFactory) {
	dir := t.TempDir()
	location := path.Join(dir, "test_artifact")

	return location, &MockArtiFactory{
		t:    t,
		data: "test data",
	}
}

func TestFetchArtifact(t *testing.T) {
	location, mockFactory := newMockArtiFactory(t)

	_, err := TryFetchArtifact(location, mockFactory)
	require.Error(t, err)

	// Create a mock artifact file
	_, raw, err := mockFactory.Generate()
	require.NoError(t, err)
	err = os.WriteFile(location, raw, 0o600)
	require.NoError(t, err)
	defer os.Remove(location)

	artifact, err := TryFetchArtifact(location, mockFactory)
	assert.NoError(t, err)
	assert.Equal(t, mockFactory.data, artifact)
}

func TestArtifactRetryBackoff(t *testing.T) {
	for _, operation := range []struct {
		name  string
		fetch func(context.Context, string, ArtifactBuilder[string]) (string, error)
	}{
		{"fetch", FetchArtifact[string]},
		{"fetch_or_create", FetchOrCreateArtifact[string]},
	} {
		t.Run(operation.name, func(t *testing.T) {
			// Publish just after the preceding attempt and check when the next
			// attempt finds the artifact. Include two retries at the 5s cap.
			var previousAttempt time.Duration
			for _, expected := range []time.Duration{
				0, 100 * time.Millisecond, 200 * time.Millisecond,
				300 * time.Millisecond, 400 * time.Millisecond,
				500 * time.Millisecond, 600 * time.Millisecond,
				700 * time.Millisecond, 800 * time.Millisecond,
				900 * time.Millisecond, 1000 * time.Millisecond,
				1200 * time.Millisecond, 1600 * time.Millisecond,
				2400 * time.Millisecond, 4000 * time.Millisecond,
				7200 * time.Millisecond, 12200 * time.Millisecond,
				17200 * time.Millisecond,
			} {
				t.Run(expected.String(), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						location, factory := newMockArtiFactory(t)
						// Prevent FetchOrCreateArtifact from creating the artifact itself.
						lock := flock.New(location + lockSuffix)
						locked, err := lock.TryLock()
						require.NoError(t, err)
						require.True(t, locked)
						defer lock.Unlock()

						if expected == 0 {
							require.NoError(t, os.WriteFile(location, []byte(factory.data), 0o600))
						} else {
							go func() {
								time.Sleep(previousAttempt + time.Millisecond)
								assert.NoError(t, os.WriteFile(location, []byte(factory.data), 0o600))
							}()
						}

						ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
						defer cancel()
						start := time.Now()
						artifact, err := operation.fetch(ctx, location, factory)
						require.NoError(t, err)
						assert.Equal(t, factory.data, artifact)
						assert.Equal(t, expected, time.Since(start))
					})
				})
				previousAttempt = expected
			}

			// Cancellation must interrupt both the fixed-delay and capped-backoff phases.
			for _, timeout := range []time.Duration{50 * time.Millisecond, 8 * time.Second} {
				t.Run("cancellation/"+timeout.String(), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						location, factory := newMockArtiFactory(t)
						lock := flock.New(location + lockSuffix)
						locked, err := lock.TryLock()
						require.NoError(t, err)
						require.True(t, locked)
						defer lock.Unlock()

						ctx, cancel := context.WithTimeout(t.Context(), timeout)
						defer cancel()
						start := time.Now()
						artifact, err := operation.fetch(ctx, location, factory)
						require.Error(t, err)
						assert.Empty(t, artifact)
						assert.Equal(t, timeout, time.Since(start))
					})
				})
			}
		})
	}
}

func TestCreateNewArtifact(t *testing.T) {
	location, mockFactory := newMockArtiFactory(t)

	artifact, err := FetchOrCreateArtifact(t.Context(), location, mockFactory)
	assert.NoError(t, err)
	assert.Equal(t, mockFactory.data, artifact)

	// Verify the artifact file was created
	content, err := os.ReadFile(location)
	require.NoError(t, err)
	loadedArtifact, _ := mockFactory.Deserialize(content)
	assert.Equal(t, mockFactory.data, loadedArtifact)

	// The lock file should be cleaned up
	lockFilePath := location + lockSuffix
	_, err = os.Stat(lockFilePath)
	require.ErrorIs(t, err, os.ErrNotExist,
		"lock file should not exist after successful creation and concurrent reads")
}

func TestContextCancellation(t *testing.T) {
	synctest.Test(t, syncTestContextCancellation)
}

func syncTestContextCancellation(t *testing.T) {
	location, mockFactory := newMockArtiFactory(t)

	// Ensure the artifact file does not exist
	os.Remove(location)

	// Create a lock file to simulate contention
	lockFile := flock.New(location + ".lock")
	isLock, err := lockFile.TryLock()
	assert.NoError(t, err)
	assert.True(t, isLock)
	defer lockFile.Unlock()

	// Create a context with a timeout to simulate cancellation
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	// Call FetchOrCreateArtifact with the context
	_, err = FetchOrCreateArtifact(ctx, location, mockFactory)

	// Check that the error is due to context cancellation
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to read the artifact or acquire the lock in the given time")
}

func TestHandleMultipleConcurrentWrites(t *testing.T) {
	synctest.Test(t, syncTestHandleMultipleConcurrentWrites)
}

func syncTestHandleMultipleConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	location := path.Join(dir, "test_artifact")

	g := new(errgroup.Group)

	// Number of concurrent goroutines
	numGoroutines := 50

	results := make(chan string, numGoroutines)

	// Start multiple goroutines to call FetchOrCreateArtifact in parallel
	for i := 0; i < numGoroutines; i++ {
		g.Go(func() error {
			generator := func() string {
				key := make([]byte, 32)
				_, err := rand.Read(key)
				assert.NoError(t, err)
				return hex.EncodeToString(key)
			}

			instance := &MockArtiFactory{
				t:             t,
				id:            i,
				dataGenerator: generator,
			}
			res, err := FetchOrCreateArtifact(t.Context(), location, instance)
			results <- res
			return err
		})
	}

	err := g.Wait()
	assert.NoError(t, err)

	// Read the first artifact
	content, err := os.ReadFile(location)
	require.NoError(t, err)
	stringContent := string(content)

	// Make sure that all goroutine produced the same output
	for i := 0; i < numGoroutines; i++ {
		readedArtifact := <-results
		assert.Equal(t, stringContent, readedArtifact, "all goroutines should read the same final artifact")
	}

	// The lock file should be cleaned up
	lockFilePath := location + lockSuffix
	_, err = os.Stat(lockFilePath)
	require.ErrorIs(t, err, os.ErrNotExist,
		"lock file should not exist after successful creation and concurrent reads")
}

func TestKeepTryingLockingIfPermissionDenied(t *testing.T) {
	synctest.Test(t, syncTestKeepTryingLockingIfPermissionDenied)
}

func syncTestKeepTryingLockingIfPermissionDenied(t *testing.T) {
	location, mockFactory := newMockArtiFactory(t)
	lockFilePath := location + lockSuffix

	// Create a lock file to simulate contention
	lockFile := flock.New(lockFilePath)
	isLock, err := lockFile.TryLock()
	assert.NoError(t, err)
	assert.True(t, isLock)
	defer lockFile.Unlock()

	// Making the lock file unreadable
	err = os.Chmod(lockFilePath, 0o000)
	require.NoError(t, err)

	// Create a context with a timeout to simulate cancellation
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	// Calling FetchOrCreateArtifact in a goroutine to simulate a concurrent call
	g := new(errgroup.Group)
	g.Go(func() error {
		_, err := FetchOrCreateArtifact(ctx, location, mockFactory)
		return err
	})

	// Wait for FetchOrCreateArtifact to try at least once to acquire the lock
	synctest.Wait()

	// Make the lock file readable again and release it
	err = os.Chmod(lockFilePath, 0o600)
	require.NoError(t, err)
	err = lockFile.Unlock()
	require.NoError(t, err)

	err = g.Wait()
	assert.NoError(t, err)

	// The lock file should be cleaned up
	_, err = os.Stat(lockFilePath)
	require.ErrorIs(t, err, os.ErrNotExist,
		"lock file should not exist after successful creation and concurrent reads")
}
