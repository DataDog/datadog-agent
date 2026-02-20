// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

// shortBackoffPolicy is a test backoff policy that returns a very short duration.
type shortBackoffPolicy struct{}

func (p *shortBackoffPolicy) GetBackoffDuration(_ int) time.Duration {
	return 10 * time.Millisecond
}

func (p *shortBackoffPolicy) IncError(numErrors int) int {
	return numErrors + 1
}

func (p *shortBackoffPolicy) DecError(numErrors int) int {
	if numErrors > 0 {
		return numErrors - 1
	}
	return 0
}

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

	symdbURL, err := url.Parse(symdbServer.URL)
	require.NoError(t, err)

	// Create process store and symdb manager
	const cacheDir = "" // no cache
	manager := newSymdbManager(symdbURL, object.NewInMemoryLoader(), cacheDir)
	t.Cleanup(manager.stop)

	// Create runtime ID for testing
	runtimeID := procRuntimeID{
		ID:          process.ID{PID: 12345},
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
	// Create a context that will be canceled when the test is done.
	// Unforunately, context cancellation doesn't propagate through http clients
	// the way it does for gRPC.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	symdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if blockUpload {
			// Notify that the upload started, and block until notified.
			select {
			case uploadSemaphore <- struct{}{}:
				// Good, upload started. We'll unblock it below.
				select {
				case <-uploadSemaphore:
				case <-ctx.Done():
				}
			case <-ctx.Done():
			}
		}
		uploadCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer symdbServer.Close()
	defer cancel() // prevent server.Close() from blocking forever

	symdbURL, err := url.Parse(symdbServer.URL)
	require.NoError(t, err)

	manager := newSymdbManager(
		symdbURL,
		object.NewInMemoryLoader(),
		"", /* cacheDir - no cache */
		// Use a small buffer (which will force a flush after every package)
		// in order to have an opportunity to cancel the uploads in between
		// flushes.
		withMaxBufferFuncs(1),
	)
	t.Cleanup(manager.stop)
	// Create a dummy runtime ID
	runtimeID := procRuntimeID{
		ID:          process.ID{PID: 12345},
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
	runtimeID.ID.PID++ // change the ID to a new process
	blockUpload = true
	err = manager.queueUpload(runtimeID, binPath)
	require.NoError(t, err)

	// Wait for upload to start (i.e., for the first HTTP request to be made).
	// The upload is asynchronous, so it may take a moment to start processing.
	select {
	case <-uploadSemaphore:
		// Good, upload started. We'll unblock it below.
	case <-time.After(30 * time.Second):
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

// Test that SymDBManager respects entries in the persistent cache which control
// which uploads should be deferred or rejected.
func TestSymdbManagerRespectsPersistentCache(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	cfg := cfgs[0] // use first config
	binPath := testprogs.MustGetBinary(t, "rc_tester", cfg)

	tests := []struct {
		name            string
		entryType       entryType
		errorNumber     int
		errorMsg        string
		timestampOffset time.Duration // Offset from current time (negative = in the past).
		expectDeferred  bool
		expectRejected  bool
	}{
		{
			// The upload should be retried after a wait, since it previously
			// failed.
			name:           "FailedUpload",
			entryType:      entryTypeAttempt,
			errorNumber:    1,
			errorMsg:       "previous upload failed",
			expectDeferred: true,
		},
		{
			// The upload should be rejected because it previously succeeded.
			name:           "CompletedUpload",
			entryType:      entryTypeCompleted,
			expectRejected: true,
		},
		{
			// The upload should be performed immediately because the backoff
			// has expired.
			name:            "FailedUploadBackoffExpired",
			entryType:       entryTypeAttempt,
			errorNumber:     1,
			errorMsg:        "previous upload failed",
			timestampOffset: -20 * time.Hour, // Backoff has expired.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary cache directory for this subtest.
			cacheDir := t.TempDir()

			pid := int32(12345)

			// Create and populate the cache with the test entry.
			cache, err := newPersistentUploadCache(cacheDir)
			require.NoError(t, err)
			if tt.entryType == entryTypeAttempt {
				err = cache.AddAttempt(pid, "test_service", "1.0.0", tt.errorNumber, tt.errorMsg)
				require.NoError(t, err)
			} else {
				err = cache.AddCompleted(pid, "test_service", "1.0.0")
				require.NoError(t, err)
			}

			// Adjust timestamp if needed.
			if tt.timestampOffset != 0 {
				entry, err := cache.GetEntry(pid)
				require.NoError(t, err)
				require.NotNil(t, entry)
				entry.Timestamp = time.Now().Add(tt.timestampOffset)
				err = cache.saveEntry(pid, *entry)
				require.NoError(t, err)
			}

			// Track whether callbacks were invoked.
			deferredCalled := false
			rejectedCalled := false
			uploadQueuedCh := make(chan struct{}, 1)

			// Build manager options.
			managerOpts := []option{
				withTestingKnobOnDeferUpload(func() {
					deferredCalled = true
				}),
				withTestingKnobOnUploadRejectedByPersistentCache(func() {
					rejectedCalled = true
				}),
				withTestingKnobOnUploadQueued(func(_ queuedUploadInfo) {
					uploadQueuedCh <- struct{}{}
				}),
				withCacheOptions(withProcessExistsCheck(func(p int) bool {
					return p == int(pid) // Test process exists.
				})),
			}

			// For failed uploads, use a short backoff policy so the retry happens quickly.
			if tt.expectDeferred {
				managerOpts = append(managerOpts, withBackoffPolicy(&shortBackoffPolicy{}), withDontAccountForElapsedTime())
			}

			uploadCh := make(chan struct{}, 1)

			// Set up mock SymDB backend.
			symdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				uploadCh <- struct{}{}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "ok"}`))
			}))
			defer symdbServer.Close()

			symdbURL, err := url.Parse(symdbServer.URL)
			require.NoError(t, err)

			manager := newSymdbManager(
				symdbURL,
				object.NewInMemoryLoader(),
				cacheDir,
				managerOpts...,
			)
			t.Cleanup(manager.stop)

			// Create runtime ID for testing.
			runtimeID := procRuntimeID{
				ID:          process.ID{PID: pid},
				service:     "test_service",
				environment: "test_env",
				version:     "1.0.0",
				runtimeID:   "dummy-runtime-id",
			}

			// Request an upload.
			err = manager.queueUpload(runtimeID, binPath)
			require.NoError(t, err)

			// Verify expectations.
			if tt.expectDeferred {
				require.True(t, deferredCalled, "expected upload to be deferred")
				require.False(t, rejectedCalled, "upload should not be rejected")

				// Wait for the upload to happen asynchronously.
				select {
				case <-uploadCh:
					// Success - upload was queued.
				case <-time.After(30 * time.Second):
					t.Fatal("expected upload to be retried")
				}
			} else if tt.expectRejected {
				require.True(t, rejectedCalled, "expected upload to be rejected")
				require.False(t, deferredCalled, "upload should not be deferred")
			} else {
				// Upload should be queued immediately.
				require.False(t, deferredCalled, "upload should not be deferred")
				require.False(t, rejectedCalled, "upload should not be rejected")

				select {
				case <-uploadQueuedCh:
				// Success - upload was queued synchronously.
				default:
					t.Fatal("expected upload to be queued immediately")
				}
			}
		})
	}
}

func TestSymdbManagerRetryOnNetworkError(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	cfg := cfgs[0]
	binPath := testprogs.MustGetBinary(t, "rc_tester", cfg)

	// Set up mock SymDB backend that fails the first time, succeeds the second.
	uploadCount := atomic.Int64{}
	symdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := uploadCount.Add(1)
		if count == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer symdbServer.Close()

	symdbURL, err := url.Parse(symdbServer.URL)
	require.NoError(t, err)

	uploadQueuedCh := make(chan queuedUploadInfo, 2)
	retryDelay := 10 * time.Millisecond

	manager := newSymdbManager(
		symdbURL,
		object.NewInMemoryLoader(),
		"", /* cacheDir - no cache */
		withTestingKnobOnUploadQueued(func(info queuedUploadInfo) {
			uploadQueuedCh <- info
		}),
		withNetworkErrorRetryDelay(retryDelay),
	)
	t.Cleanup(manager.stop)

	runtimeID := procRuntimeID{
		ID:          process.ID{PID: 12345},
		service:     "test_service",
		environment: "test_env",
		version:     "1.0.0",
		runtimeID:   "dummy-runtime-id",
	}

	// Request an upload.
	err = manager.queueUpload(runtimeID, binPath)
	require.NoError(t, err)

	// Wait for initial queue notification.
	select {
	case <-uploadQueuedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("initial upload was not queued")
	}

	// Wait for the retry to be scheduled (after the upload fails).
	var retryInfo queuedUploadInfo
	select {
	case retryInfo = <-uploadQueuedCh:
	case <-time.After(30 * time.Second):
		t.Fatal("retry was not scheduled after network error")
	}
	afterRetryScheduled := time.Now()

	// Verify the first upload was attempted.
	require.Equal(t, int64(1), uploadCount.Load(), "expected 1 upload attempt")

	// Verify the retry is scheduled in the future by approximately the
	// configured delay. Allow some tolerance since the scheduled time is
	// computed before we receive the notification.
	expectedRetryTime := afterRetryScheduled.Add(retryDelay / 2)
	require.WithinDuration(t, expectedRetryTime, retryInfo.ScheduledTime, time.Second,
		"retry should be scheduled with the configured delay")

	// Wait for the retry to actually happen.
	require.Eventually(t, func() bool {
		return uploadCount.Load() == 2
	}, 10*time.Second, 10*time.Millisecond, "expected retry upload to succeed")

	// Verify the queue is now empty.
	require.Eventually(t, func() bool {
		return manager.queueSize() == 0
	}, 10*time.Second, 10*time.Millisecond, "expected queue to be empty after successful retry")
}
