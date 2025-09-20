// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestSymdbManagerUpload(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	cfg := cfgs[0] // use first config
	binPath := testprogs.MustGetBinary(t, "rc_tester", cfg)

	// Set up mock SymDB backend.
	uploadCount := atomic.Int64{}
	var lastUploadData []byte
	symdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Read the uploaded data
		data := make([]byte, r.ContentLength)
		_, err := r.Body.Read(data)
		if err != nil && err.Error() != "EOF" {
			t.Errorf("Failed to read upload data: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		lastUploadData = data
		uploadCount.Add(1)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer symdbServer.Close()

	// Parse server URL for manager
	symdbURL, err := url.Parse(symdbServer.URL)
	require.NoError(t, err)

	// Create process store and symdb manager
	manager := newSymdbManager(symdbURL, object.NewInMemoryLoader())
	t.Cleanup(manager.stop)

	// Create runtime ID for testing
	runtimeID := procRuntimeID{
		ProcessID:   procmon.ProcessID{PID: 12345},
		service:     "test_service",
		environment: "test_env",
		version:     "1.0.0",
		runtimeID:   "dummy-runtime-id",
	}

	// Request an upload.
	err = manager.queueUpload(runtimeID, binPath)
	require.NoError(t, err)

	// Wait for upload to complete, as reported by the manager.
	require.Eventually(t, func() bool {
		return manager.queueSize() == 0
	}, 10*time.Second, 100*time.Millisecond, "expected upload to complete")

	// Verify upload occurred.
	require.Greater(t, uploadCount.Load(), int64(0), "No uploads received")
	require.NotEmpty(t, lastUploadData, "No upload data received")

	// Ask for another upload for the same process and check that we do not
	// actually perform the upload.
	initialCount := uploadCount.Load()
	err = manager.queueUpload(runtimeID, binPath)
	require.NoError(t, err)

	// Wait for upload to complete.
	require.Eventually(t, func() bool {
		return manager.queueSize() == 0
	}, 10*time.Second, 100*time.Millisecond, "expected upload to complete")

	require.Equal(t, uploadCount.Load(), initialCount, "second upload occurred unexpectedly")
}

func TestSymdbManagerCancellation(t *testing.T) {
	t.Run("CancelViaRemoveUpload", func(t *testing.T) {
		testSymdbManagerCancellation(t, false /* useStop */)
	})

	t.Run("CancelViaStop", func(t *testing.T) {
		testSymdbManagerCancellation(t, true /* useStop */)
	})
}

func testSymdbManagerCancellation(t *testing.T, useStop bool) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	cfg := cfgs[0] // use first config
	binPath := testprogs.MustGetBinary(t, "rc_tester", cfg)

	uploadSemaphore := make(chan struct{})
	blockUpload := false
	uploadCount := 0
	symdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if blockUpload {
			// Notify that the upload started, and block until notified.
			uploadSemaphore <- struct{}{}
			<-uploadSemaphore
		}
		uploadCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer symdbServer.Close()

	symdbURL, err := url.Parse(symdbServer.URL)
	require.NoError(t, err)

	manager := newSymdbManager(
		symdbURL,
		object.NewInMemoryLoader(),
		// Use a small buffer (which will force a flush after every package)
		// in order to have an opportunity to cancel the uploads in between
		// flushes.
		withMaxBufferFuncs(1),
	)
	t.Cleanup(manager.stop)
	// Create a dummy runtime ID
	runtimeID := procRuntimeID{
		ProcessID:   procmon.ProcessID{PID: 12345},
		service:     "test_service",
		environment: "test_env",
		version:     "1.0.0",
		runtimeID:   "dummy-runtime-id",
	}

	// First perform an upload, which we won't block or cancel, to see how many
	// HTTP requests we get. We need to make sure that we get more than one, in
	// order for the cancellation we perform afterward to make sense.
	err = manager.queueUpload(runtimeID, binPath)
	require.NoError(t, err)

	// Wait for upload to complete, as reported by the manager.
	require.Eventually(t, func() bool {
		return manager.queueSize() == 0
	}, 10*time.Second, 100*time.Millisecond, "expected upload to complete")
	// We expect each package to be uploaded as a separate HTTP request. There
	// should be a bunch of packages; the second part of this test only makes
	// sense if there is more than one package.
	require.Greater(t, uploadCount, 1, "multiple HTTP requests were expected")
	uploadCount = 0

	// Now perform another upload (for a different process), which we'll block
	// and then cancel.
	runtimeID.ProcessID.PID++ // change the ID to a new process
	blockUpload = true
	err = manager.queueUpload(runtimeID, binPath)
	require.NoError(t, err)
	select {
	case <-uploadSemaphore:
		t.Fatal("Upload started unexpectedly")
	case <-time.After(100 * time.Millisecond):
		// OK, no upload.
	}

	// Wait for upload to start.
	select {
	case <-uploadSemaphore:
		// Good, upload started. We'll unblock it below.
	case <-time.After(2 * time.Second):
		t.Fatal("Upload did not start in time")
	}

	// Now cancel the upload using the specified method.
	if useStop {
		manager.stop()
	} else {
		manager.removeUpload(runtimeID)
	}
	// Unblock the first HTTP request. There should be no more HTTP requests.
	uploadSemaphore <- struct{}{}
	// Wait a little bit to make sure there are no more requests.
	select {
	case <-uploadSemaphore:
		t.Fatal("Upload was not cancelled")
	case <-time.After(100 * time.Millisecond):
		// Good, no more requests.
	}

	if useStop {
		// Check that the manager actually stopped and doesn't process more
		// uploads. Try to queue another upload after stop - it should be
		// rejected.
		err := manager.queueUpload(runtimeID, binPath)
		require.ErrorContains(t, err, "stopped")
	}
}
