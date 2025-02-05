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
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/rand"
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
	t.Parallel()
	location, mockFactory := newMockArtiFactory(t)

	_, err := FetchArtifact(location, mockFactory)
	require.Error(t, err)

	// Create a mock artifact file
	_, raw, err := mockFactory.Generate()
	require.NoError(t, err)
	err = os.WriteFile(location, raw, 0o600)
	require.NoError(t, err)
	defer os.Remove(location)

	artifact, err := FetchArtifact(location, mockFactory)
	assert.NoError(t, err)
	assert.Equal(t, mockFactory.data, artifact)
}

func TestCreateNewArtifact(t *testing.T) {
	t.Parallel()
	location, mockFactory := newMockArtiFactory(t)

	artifact, err := FetchOrCreateArtifact(context.Background(), location, mockFactory)
	assert.NoError(t, err)
	assert.Equal(t, mockFactory.data, artifact)

	// Verify the artifact file was created
	content, err := os.ReadFile(location)
	require.NoError(t, err)
	loadedArtifact, _ := mockFactory.Deserialize(content)
	assert.Equal(t, mockFactory.data, loadedArtifact)
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()
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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Call FetchOrCreateArtifact with the context
	_, err = FetchOrCreateArtifact(ctx, location, mockFactory)

	// Check that the error is due to context cancellation
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to acquire lock in the given time")
}

func TestHandleMultipleConcurrentWrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	location := path.Join(dir, "test_artifact")

	g := new(errgroup.Group)

	// Number of concurrent goroutines
	numGoroutines := 50

	results := make(chan string, numGoroutines)

	// Start multiple goroutines to call LoadOrCreateArtifact in parallel
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
			res, err := FetchOrCreateArtifact(context.Background(), location, instance)
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
	require.True(t, os.IsNotExist(err),
		"lock file should not exist after successful creation and concurrent reads")
}
