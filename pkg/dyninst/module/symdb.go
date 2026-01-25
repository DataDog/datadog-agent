// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/google/uuid"
)

// symdbManager deals with uploading symbols to the SymDB backend.
type symdbManager struct {
	// If enabled is false, we never upload anything.
	enabled      bool
	uploadURL    *url.URL
	objectLoader object.Loader
	cfg          symdbManagerConfig

	// The cache persisting information about uploads across agent restarts. Nil
	// if the cache could not be created.
	persistentCache *persistentUploadCache

	// Channel for notifications for modifications to mu.queuedUploads.
	workNotification chan struct{}

	mu struct {
		sync.Mutex
		queuedUploads []scheduledUpload
		currentUpload *uploadRequest // nil if no upload is in progress
		currentCancel context.CancelCauseFunc
		currentDone   chan struct{} // closed when current upload finishes
		stopped       bool

		// trackedProcesses stores processes for which an upload was requested
		// (and perhaps completed). Repeated requests for the same process
		// (through queueProcess()) are no-ops.
		//
		// Note that uploads are also tracked in persistentCache (if we're
		// configured with a persistent cache), in addition to trackedProcesses.
		// This double tracking is redundant, except the cache is not always
		// present.
		trackedProcesses map[processKey]struct{}
	}
	workerWg     sync.WaitGroup
	workerCancel context.CancelCauseFunc
}

type scheduledUpload struct {
	scheduledTime time.Time
	req           uploadRequest
}

type symdbManagerInterface interface {
	// queueUpload queues a new upload request, if the process' data has not
	// been uploaded previously. Calling queueUpload() again for the same
	// process is a no-op.
	//
	// The upload will be performed asynchronously. A single upload can be in
	// progress at a time. Returns an error if the manager has been stopped.
	queueUpload(runtimeID procRuntimeID, executablePath string) error
	// removeUpload removes the queued upload for the given process ID, if any. If
	// the upload is currently being processed, it will be cancelled and the call
	// will block until the upload stops. If no upload for the respective process is
	// in progress or queued, the call is a no-op.
	removeUpload(runtimeID procRuntimeID)
	// removeUploadByPID removes the queued upload(s) for the given process ID.
	removeUploadByPID(pid process.ID)
}

type processKey struct {
	pid     process.ID
	service string
	version string
}

func (k processKey) String() string {
	return fmt.Sprintf("%s (service: %s)", k.pid, k.service)
}

type uploadRequest struct {
	procID         processKey
	runtimeID      string
	executablePath string
}

// newSymdbManager creates a new symdbManager and starts the upload worker.
//
// If uploadURL is nil, the manager is disabled and no uploads are performed.
//
// If cacheDir is not empty, the path will be used to track the status of SymDB
// uploads across agent restarts.
func newSymdbManager(
	uploadURL *url.URL,
	objectLoader object.Loader,
	cacheDir string,
	opts ...option,
) *symdbManager {
	cfg := symdbManagerConfig{
		maxBufferFuncs: 10000,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	workerCtx, cancel := context.WithCancelCause(context.Background())
	m := &symdbManager{
		enabled:      uploadURL != nil,
		uploadURL:    uploadURL,
		objectLoader: objectLoader,
		cfg:          cfg,
		workerCancel: cancel,
		// Channel with a buffer of one, so that a notifier can leave a
		// notification even if the worker is not yet reading from the channel
		// (to avoid a race where the worker checks the works queue, releases
		// the lock, and misses a notification that happened before it starts
		// waiting on the channel).
		workNotification: make(chan struct{}, 1),
	}
	m.mu.trackedProcesses = make(map[processKey]struct{})

	if cacheDir != "" {
		cache, err := newPersistentUploadCache(cacheDir, cfg.testingKnobs.cacheOptions...)
		if err != nil {
			log.Errorf("Failed to create persistent upload cache dir %s: %v", cacheDir, err)
		} else {
			m.persistentCache = cache
		}
	}

	// Start the worker goroutine with tracking.
	m.workerWg.Add(1)
	go func() {
		m.worker(workerCtx)
		m.workerWg.Done()
	}()
	return m
}

type symdbManagerConfig struct {
	maxBufferFuncs int
	testingKnobs   struct {
		onDeferUpload                     func()
		onUploadRejectedByPersistentCache func()
		onUploadQueued                    func(queuedUploadInfo)
		cacheOptions                      []cacheOption
		backoffPolicy                     backoff.Policy
		dontAccountForElapsedTime         bool
		networkErrorRetryDelay            time.Duration
	}
}

type option func(config *symdbManagerConfig)

func withMaxBufferFuncs(maxBufferFuncs int) option {
	return func(c *symdbManagerConfig) {
		c.maxBufferFuncs = maxBufferFuncs
	}
}

func withTestingKnobOnDeferUpload(onDeferUpload func()) option {
	return func(c *symdbManagerConfig) {
		c.testingKnobs.onDeferUpload = onDeferUpload
	}
}

func withTestingKnobOnUploadRejectedByPersistentCache(onUploadRejectedByPersistentCache func()) option {
	return func(c *symdbManagerConfig) {
		c.testingKnobs.onUploadRejectedByPersistentCache = onUploadRejectedByPersistentCache
	}
}

func withCacheOptions(opts ...cacheOption) option {
	return func(c *symdbManagerConfig) {
		c.testingKnobs.cacheOptions = opts
	}
}

func withBackoffPolicy(policy backoff.Policy) option {
	return func(c *symdbManagerConfig) {
		c.testingKnobs.backoffPolicy = policy
	}
}

// queuedUploadInfo contains information about a queued upload, for testing.
type queuedUploadInfo struct {
	Request       uploadRequest
	ScheduledTime time.Time
}

func withTestingKnobOnUploadQueued(onUploadQueued func(queuedUploadInfo)) option {
	return func(c *symdbManagerConfig) {
		c.testingKnobs.onUploadQueued = onUploadQueued
	}
}

func withDontAccountForElapsedTime() option {
	return func(c *symdbManagerConfig) {
		c.testingKnobs.dontAccountForElapsedTime = true
	}
}

func withNetworkErrorRetryDelay(d time.Duration) option {
	return func(c *symdbManagerConfig) {
		c.testingKnobs.networkErrorRetryDelay = d
	}
}

// stop gracefully shuts down the symdbManager, cancelling any ongoing uploads
// and waiting for the worker goroutine to exit.
func (m *symdbManager) stop() {
	m.mu.Lock()
	m.mu.stopped = true
	m.mu.Unlock()
	// Cancel the worker context to signal shutdown.
	m.workerCancel(errors.New("SymDB manager shutting down"))
	// Wait for the worker to terminate.
	m.workerWg.Wait()
}

// queueUpload queues a new upload request, if the process' data has not been
// uploaded previously. Calling queueUpload() again for the same process is a
// no-op.
//
// The upload will be performed asynchronously. A single upload can be in
// progress at a time. Returns an error if the manager has been stopped.
func (m *symdbManager) queueUpload(runtimeID procRuntimeID, executablePath string) error {
	if !m.enabled {
		return nil
	}

	// Queue the upload request. The worker will pick it up.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Reject calls after Stop() has been called.
	if m.mu.stopped {
		return errors.New("symdbManager has been stopped")
	}

	// If we've already uploaded data for this process (or it's currently in the
	// queue), there's nothing to do.
	// NOTE: we identify processes not just by PID, by also by service/version.
	// If we get multiple requests for uploads with the same pid, but different
	// service/version, we'll upload multiple times with the difference
	// service/version tags.
	key := processKey{
		pid:     runtimeID.ID,
		service: runtimeID.service,
		version: runtimeID.version,
	}
	if _, ok := m.mu.trackedProcesses[key]; ok {
		// We're already tracking this process, nothing to do.
		return nil
	}
	// Track this process so that we don't upload it multiple times.
	m.mu.trackedProcesses[key] = struct{}{}

	// Check if we already have performed, or attempted to perform, an upload
	// for this process before. If we have and the attempt succeeded, there's
	// nothing to do. If it failed, we only retry it according to an exponential
	// backoff policy.
	if m.persistentCache != nil {
		entry, err := m.persistentCache.GetEntry(runtimeID.ID.PID)
		if err != nil {
			log.Warnf("failed to check upload cache for process %s", runtimeID.String())
		}
		if entry != nil && entry.ServiceName == runtimeID.service && entry.ServiceVersion == runtimeID.version {
			switch entry.Type {
			case entryTypeCompleted:
				// We already uploaded symbols for this process before the agent
				// restarted. Nothing more to do, now that we added the process
				// to m.mu.trackedProcesses.
				if m.cfg.testingKnobs.onUploadRejectedByPersistentCache != nil {
					m.cfg.testingKnobs.onUploadRejectedByPersistentCache()
				}
				log.Debugf("skipping SymDB upload for %s (%s) because it was previously uploaded", runtimeID.service, runtimeID.ID)
				return nil
			case entryTypeAttempt:
				// We already attempted to upload symbols for this process
				// before the agent restarted. We'll retry it according to an
				// exponential backoff policy.
				policy := m.cfg.testingKnobs.backoffPolicy
				if policy == nil {
					policy = backoff.NewExpBackoffPolicy(
						2,
						600,    /* baseBackoffTime - start at 10 minutes */
						2*3600, /* maxBackoffTime */
						0, false /* recoveryReset */)
				}
				backoffDuration := policy.GetBackoffDuration(entry.ErrorNumber)
				elapsed := time.Since(entry.Timestamp)
				remainingWait := backoffDuration - elapsed
				if m.cfg.testingKnobs.dontAccountForElapsedTime {
					remainingWait = backoffDuration
				}
				if remainingWait < 0 {
					// The backoff duration has expired. Allow the upload to proceed.
					break
				}
				log.Infof("SymDB: scheduling retry upload for process %s after backoff of %v (error number %d, elapsed since last attempt: %s)",
					runtimeID.ID, remainingWait, entry.ErrorNumber, elapsed)

				if m.cfg.testingKnobs.onDeferUpload != nil {
					m.cfg.testingKnobs.onDeferUpload()
				}

				// Schedule the retry after the remaining backoff duration.
				m.addToQueueLocked(uploadRequest{
					procID:         key,
					runtimeID:      runtimeID.runtimeID,
					executablePath: executablePath,
				}, time.Now().Add(remainingWait))

				return nil
			default:
				log.Warnf("invalid entry type in cache for process %s: %d", runtimeID, entry.Type)
			}
		}
	}

	m.addToQueueLocked(uploadRequest{
		procID:         key,
		runtimeID:      runtimeID.runtimeID,
		executablePath: executablePath,
	}, time.Now())

	return nil
}

func (m *symdbManager) addToQueue(req uploadRequest, scheduledTime time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addToQueueLocked(req, scheduledTime)
}

func (m *symdbManager) addToQueueLocked(req uploadRequest, scheduledTime time.Time) {
	m.mu.queuedUploads = append(m.mu.queuedUploads, scheduledUpload{
		req:           req,
		scheduledTime: scheduledTime,
	})
	m.notifyWorker()
	if m.cfg.testingKnobs.onUploadQueued != nil {
		m.cfg.testingKnobs.onUploadQueued(queuedUploadInfo{
			Request:       req,
			ScheduledTime: scheduledTime,
		})
	}
}

// Signal the worker that a new request is available.
func (m *symdbManager) notifyWorker() {
	select {
	case m.workNotification <- struct{}{}:
	default:
		// The channel is already full; nothing to do.
	}
}

// removeUpload removes the queued upload for the given process ID, if any. If
// the upload is currently being processed, it will be cancelled and the call
// will block until the upload stops. Future calls to queueUpload() for the
// respective process will result in a new upload being queued.
func (m *symdbManager) removeUpload(runtimeID procRuntimeID) {
	key := processKey{
		pid:     runtimeID.ID,
		service: runtimeID.service,
		version: runtimeID.version,
	}
	m.removeUploadInner(key)
}

func (m *symdbManager) removeUploadInner(key processKey) {
	m.mu.Lock()

	if _, ok := m.mu.trackedProcesses[key]; !ok {
		m.mu.Unlock()
		return
	}
	delete(m.mu.trackedProcesses, key)
	if m.persistentCache != nil {
		if err := m.persistentCache.RemoveEntry(key.pid.PID); err != nil {
			log.Warnf("failed to remove cache entry for process %s", key.pid)
		}
	}

	// Remove future uploads from queue.
	filtered := m.mu.queuedUploads[:0]
	for _, req := range m.mu.queuedUploads {
		if req.req.procID != key {
			filtered = append(filtered, req)
		}
	}
	m.mu.queuedUploads = filtered

	// Deal with the case where the current upload is being removed: cancel and
	// wait for it to terminate.
	var doneCh chan struct{}
	if m.mu.currentUpload != nil && m.mu.currentUpload.procID == key {
		log.Infof("Cancelling symbols upload for process %s", key)
		// Cancel the upload first.
		if m.mu.currentCancel != nil {
			m.mu.currentCancel(errors.New("symbols upload no longer required"))
		}
		// We'll wait for the upload to finish (or, rather, to respond to the
		// cancellation) without holding the lock.
		doneCh = m.mu.currentDone
	}
	m.mu.Unlock()

	// Wait for upload to finish without holding the lock.
	if doneCh != nil {
		<-doneCh
	}
}

// removeUploadByPID removes the queued upload(s) for the given process ID.
func (m *symdbManager) removeUploadByPID(pid process.ID) {
	// Find all the requests with this pid -- in theory there could be more than one, with
	// different service/version tags.
	m.mu.Lock()
	var keys []processKey
	for k := range m.mu.trackedProcesses {
		if k.pid == pid {
			keys = append(keys, k)
		}
	}
	m.mu.Unlock()

	for _, k := range keys {
		m.removeUploadInner(k)
	}
}

func (m *symdbManager) queueSize() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	inProgress := 0
	if m.mu.currentUpload != nil {
		inProgress = 1
	}
	return len(m.mu.queuedUploads) + inProgress
}

// worker processes upload requests from the queue one by one. Runs until ctx is
// canceled.
func (m *symdbManager) worker(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a timer once and reuse it throughout the loop.
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		timer.Stop() // We'll recompute the timer below.

		// Find the upload with the earliest scheduled time and check if it's ready.
		earliestIdx := -1
		workReady := false
		if len(m.mu.queuedUploads) > 0 {
			earliestIdx = 0
			earliestTime := m.mu.queuedUploads[0].scheduledTime
			for i := 1; i < len(m.mu.queuedUploads); i++ {
				if m.mu.queuedUploads[i].scheduledTime.Before(earliestTime) {
					earliestIdx = i
					earliestTime = m.mu.queuedUploads[i].scheduledTime
				}
			}
			// Check if the earliest upload is ready to be processed.
			workReady = !time.Now().Before(earliestTime)
		}

		// If there's no work ready, wait for either new work to come in, or the
		// timer of the earliest scheduled upload to expire.
		if !workReady {
			// If we have work scheduled for the future, reset the timer. If we
			// don't have work in the future, the read from timer.C below will
			// block indefinitely.
			if earliestIdx >= 0 {
				earliestTime := m.mu.queuedUploads[earliestIdx].scheduledTime
				timer.Reset(time.Until(earliestTime))
			}

			m.mu.Unlock()
			select {
			case <-m.workNotification:
			case <-timer.C:
			case <-ctx.Done():
			}
			m.mu.Lock()
			if ctx.Err() != nil {
				return
			}

			// Loop back to re-check the queue.
			continue
		}

		// Take the upload with the earliest time from the queue.
		upload := m.mu.queuedUploads[earliestIdx]
		m.mu.queuedUploads = append(m.mu.queuedUploads[:earliestIdx], m.mu.queuedUploads[earliestIdx+1:]...)

		req := upload.req
		uploadCtx, cancel := context.WithCancelCause(ctx)
		m.mu.currentUpload = &req
		m.mu.currentCancel = cancel
		m.mu.currentDone = make(chan struct{})

		// Unlock while processing the upload.
		m.mu.Unlock()
		err := m.performUpload(uploadCtx, req.procID, req.runtimeID, req.executablePath)
		if err != nil {
			if uploadCtx.Err() != nil {
				log.Infof("SymDB: upload cancelled for process %v (executable: %s): %v",
					req.runtimeID, req.executablePath, context.Cause(uploadCtx))
			} else {
				// We'll retry the upload on network errors, but not on other errors.
				var ue uploadError
				networkError := errors.As(err, &ue)
				if networkError {
					retryDelay := time.Hour
					if m.cfg.testingKnobs.networkErrorRetryDelay > 0 {
						retryDelay = m.cfg.testingKnobs.networkErrorRetryDelay
					}
					nextAttempt := time.Now().Add(retryDelay)
					log.Errorf("SymDB: failed to upload symbols for process %v (executable: %s): %v. Will be attempted again at: %s.",
						req.procID.pid, req.executablePath, err, nextAttempt)
					m.addToQueue(req, nextAttempt)
				} else {
					log.Errorf("SymDB: failed to upload symbols for process %v (executable: %s): %v. It will not be attempted again.",
						req.procID.pid, req.executablePath, err)
				}
			}
		}

		// Re-acquire lock and clear current upload.
		m.mu.Lock()
		m.mu.currentUpload = nil
		m.mu.currentCancel = nil
		close(m.mu.currentDone)
		m.mu.currentDone = nil
	}
}

func (m *symdbManager) performUpload(
	ctx context.Context, procID processKey, runtimeID string, executablePath string,
) (retErr error) {
	// Create or update the cache entry to track this upload attempt.
	if m.persistentCache != nil {
		errorNumber := 1
		// Check if an entry already exists. If it does, we'll increment the
		// error number.
		entry, err := m.persistentCache.GetEntry(procID.pid.PID)
		if err != nil {
			return fmt.Errorf("failed to read cache entry for process %v: %w", procID.pid, err)
		}
		if entry != nil && entry.Type == entryTypeAttempt {
			errorNumber = entry.ErrorNumber + 1
		}

		// Record this attempt in the cache.
		if err := m.persistentCache.AddAttempt(procID.pid.PID, procID.service, procID.version, errorNumber, "" /* errMsg */); err != nil {
			return fmt.Errorf("failed to create cache entry for process %v: %w", procID.pid, err)
		}

		// Update the cache entry when the upload is complete.
		defer func() {
			if retErr == nil {
				// Upload succeeded, mark it as completed. If the agent
				// restarts, it will use this completed entry to avoid uploading
				// the data again for this process.
				if err := m.persistentCache.AddCompleted(
					procID.pid.PID, procID.service, procID.version,
				); err != nil {
					retErr = fmt.Errorf("failed to update cache entry for process %v: %w", procID.pid, err)
				}
			} else {
				// Upload failed, update the cache entry we created above with
				// the error message.
				if err := m.persistentCache.AddAttempt(
					procID.pid.PID, procID.service, procID.version, errorNumber, retErr.Error(),
				); err != nil {
					log.Errorf("Failed to update cache entry with error for process %v: %v", procID.pid, err)
				}
			}
		}()
	}

	log.Infof("SymDB: uploading symbols for process %v (service: %s, version: %s, executable: %s)",
		procID.pid, procID.service, procID.version, executablePath)
	startTime := time.Now()
	it, err := symdb.PackagesIterator(
		executablePath,
		m.objectLoader,
		symdb.ExtractOptions{Scope: symdb.ExtractScopeModulesFromSameOrg})
	if err != nil {
		return fmt.Errorf("failed to read symbols for process %v (executable: %s): %w",
			procID.pid, executablePath, err)
	}

	sender := uploader.NewSymDBUploader(
		m.uploadURL.String(),
		procID.service, procID.version, runtimeID,
	)
	uploadBuffer := make([]uploader.Scope, 0, 100)
	bufferFuncs := 0
	uploadID := uuid.New()
	batchNum := 0
	var totalPackages, totalFuncs int
	// Flush every so often in order to not store too many scopes in memory.
	maybeFlush := func(final bool) error {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}

		if len(uploadBuffer) == 0 {
			return nil
		}
		if final || bufferFuncs >= m.cfg.maxBufferFuncs {
			log.Tracef("SymDB: uploading symbols chunk: %d packages, %d functions. Final chunk: %t", len(uploadBuffer), bufferFuncs, final)
			batchNum++
			err := sender.UploadBatch(ctx,
				uploader.UploadInfo{
					UploadID: uploadID,
					BatchNum: batchNum,
					Final:    final,
				},
				uploadBuffer,
			)
			if err != nil {
				return uploadError{cause: err}
			}
			uploadBuffer = uploadBuffer[:0]
			bufferFuncs = 0
		}
		return nil
	}
	for pkg, err := range it {
		if err != nil {
			return fmt.Errorf("failed to iterate packages for process %v (executable: %s): %w",
				procID.pid, executablePath, err)
		}

		if ctx.Err() != nil {
			return context.Cause(ctx)
		}

		scope := uploader.ConvertPackageToScope(pkg.Package, version.AgentVersion)
		uploadBuffer = append(uploadBuffer, scope)
		totalPackages++
		totalFuncs += pkg.Stats().NumFunctions
		bufferFuncs += pkg.Stats().NumFunctions
		if err := maybeFlush(pkg.Final); err != nil {
			return err
		}
	}

	log.Infof("SymDB: Successfully uploaded symbols for process %v "+
		"(service: %s, version: %s, executable: %s):"+
		" %d packages, %d functions, %d chunks in %v",
		procID.pid, procID.service, procID.version, executablePath,
		totalPackages, totalFuncs, batchNum, time.Since(startTime))
	return nil
}

// uploadError represents network errors that occurred while uploading symbols.
type uploadError struct {
	cause error
}

func (u uploadError) Error() string {
	return fmt.Sprintf("upload failed: %s", u.cause)
}

func (u uploadError) Cause() error {
	return u.cause
}

var _ error = uploadError{}
