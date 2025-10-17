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

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// symdbManager deals with uploading symbols to the SymDB backend.
type symdbManager struct {
	// If enabled is false, we never upload anything.
	enabled      bool
	uploadURL    *url.URL
	objectLoader object.Loader
	cfg          symdbManagerConfig
	mu           struct {
		sync.Mutex
		queuedUploads []uploadRequest
		cond          *sync.Cond
		currentUpload *uploadRequest // nil if no upload is in progress
		currentCancel context.CancelCauseFunc
		currentDone   chan struct{} // closed when current upload finishes
		stopped       bool

		// trackedProcesses stores processes for which an upload was requested.
		// Repeated requests for the same process (through queueProcess()) are
		// no-ops. However, untrackProcess() can be called to remove a process
		// from this map, making future queueProcess() calls for it actually
		// upload data again.
		trackedProcesses map[processKey]struct{}
	}
	workerWg     sync.WaitGroup
	workerCancel context.CancelCauseFunc
}

type symdbManagerInterface interface {
	queueUpload(runtimeID procRuntimeID, executablePath string) error
	removeUpload(runtimeID procRuntimeID)
	removeUploadByPID(pid procmon.ProcessID)
}

type processKey struct {
	pid     procmon.ProcessID
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
func newSymdbManager(
	uploadURL *url.URL,
	objectLoader object.Loader,
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
	}
	m.mu.cond = sync.NewCond(&m.mu.Mutex)
	m.mu.trackedProcesses = make(map[processKey]struct{})

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
}

type option func(config *symdbManagerConfig)

func withMaxBufferFuncs(maxBufferFuncs int) option {
	return func(c *symdbManagerConfig) {
		c.maxBufferFuncs = maxBufferFuncs
	}
}

// stop gracefully shuts down the symdbManager, cancelling any ongoing uploads
// and waiting for the worker goroutine to exit.
func (m *symdbManager) stop() {
	// Cancel the worker context to signal shutdown
	m.workerCancel(errors.New("SymDB manager shutting down"))

	// Signal the worker in case it's waiting on the condition variable
	m.mu.Lock()
	m.mu.stopped = true
	m.mu.cond.Signal()
	m.mu.Unlock()

	// Wait for the worker to terminate.
	m.workerWg.Wait()
}

// queueUpload queues a new upload request, if the process' data has not been
// uploaded previously (since the last corresponding removeUpload() call, if
// any). Calling queueUpload() again for the same process is a no-op.
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
		pid:     runtimeID.ProcessID,
		service: runtimeID.service,
		version: runtimeID.version,
	}
	if _, ok := m.mu.trackedProcesses[key]; ok {
		return nil
	}
	m.mu.trackedProcesses[key] = struct{}{}

	m.mu.queuedUploads = append(m.mu.queuedUploads, uploadRequest{
		procID:         key,
		runtimeID:      runtimeID.runtimeID,
		executablePath: executablePath,
	})
	// Signal the worker that a new request is available.
	m.mu.cond.Signal()
	return nil
}

// removeUpload removes the queued upload for the given process ID, if any. If
// the upload is currently being processed, it will be cancelled and the call
// will block until the upload stops. If no upload for the respective process is
// in progress or queued, the call is a no-op.
func (m *symdbManager) removeUpload(runtimeID procRuntimeID) {
	key := processKey{
		pid:     runtimeID.ProcessID,
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

	// Remove future uploads from queue.
	filtered := m.mu.queuedUploads[:0]
	for _, req := range m.mu.queuedUploads {
		if req.procID != key {
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

	// Wait for upload to finish.
	if doneCh != nil {
		<-doneCh
	}
}

// removeUploadByPID removes the queued upload(s) for the given process ID.
func (m *symdbManager) removeUploadByPID(pid procmon.ProcessID) {
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

// worker processes upload requests from the queue one by one.
func (m *symdbManager) worker(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for {
		// Check if we should shutdown
		if ctx.Err() != nil {
			return
		}

		// Wait for work.
		for len(m.mu.queuedUploads) == 0 {
			// Check for shutdown before waiting
			if ctx.Err() != nil {
				return
			}
			m.mu.cond.Wait()
		}

		// Take the first request from the queue.
		req := m.mu.queuedUploads[0]
		m.mu.queuedUploads = m.mu.queuedUploads[1:]

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
				log.Errorf("SymDB: failed to upload symbols for process %v (executable: %s): %v",
					req.procID.pid, req.executablePath, err)
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

func (m *symdbManager) performUpload(ctx context.Context, procID processKey, runtimeID string, executablePath string) error {
	log.Infof("SymDB: uploading symbols for process %v (service: %s, version: %s, executable: %s)",
		procID.pid, procID.service, procID.version, executablePath)
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
	// Flush every so ofter in order to not store too many scopes in memory.
	maybeFlush := func(force bool) error {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}

		if len(uploadBuffer) == 0 {
			return nil
		}
		if force || bufferFuncs >= m.cfg.maxBufferFuncs {
			log.Tracef("SymDB: uploading symbols chunk: %d packages, %d functions", len(uploadBuffer), bufferFuncs)
			if err := sender.Upload(ctx, uploadBuffer); err != nil {
				return fmt.Errorf("upload failed: %w", err)
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

		scope := uploader.ConvertPackageToScope(pkg)
		uploadBuffer = append(uploadBuffer, scope)
		bufferFuncs += pkg.Stats().NumFunctions
		if err := maybeFlush(false /* force */); err != nil {
			return err
		}
	}
	if err := maybeFlush(true /* force */); err != nil {
		return err
	}

	log.Infof("SymDB: Successfully uploaded symbols for process %v (service: %s, version: %s, executable: %s)",
		procID.pid, procID.service, procID.version, executablePath)
	return nil
}
