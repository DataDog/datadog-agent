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

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// symdbManager deals with uploading symbols to the SymDB backend.
type symdbManager struct {
	uploadURL *url.URL
	cfg       symdbManagerConfig
	mu        struct {
		sync.Mutex
		queuedUploads []uploadRequest
		cond          *sync.Cond
		currentUpload *uploadRequest // nil if no upload is in progress
		currentCancel context.CancelCauseFunc
		currentDone   chan struct{} // closed when current upload finishes
		stopped       bool
	}
	workerWg     sync.WaitGroup
	workerCancel context.CancelCauseFunc
}

type uploadRequest struct {
	runtimeID      procRuntimeID
	executablePath string
}

// newSymdbManager creates a new symdbManager and starts the upload worker.
func newSymdbManager(uploadURL *url.URL, opts ...option) *symdbManager {
	cfg := symdbManagerConfig{
		maxBufferFuncs: 10000,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	workerCtx, cancel := context.WithCancelCause(context.Background())
	m := &symdbManager{
		uploadURL:    uploadURL,
		cfg:          cfg,
		workerCancel: cancel,
	}
	m.mu.cond = sync.NewCond(&m.mu.Mutex)

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

// queueUpload queues a new upload request. The upload will be performed
// asynchronously. A single upload can be in progress at a time.
// Returns an error if the manager has been stopped.
func (m *symdbManager) queueUpload(runtimeID procRuntimeID, executablePath string) error {
	// Queue the upload request. The worker will pick it up.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Reject calls after Stop() has been called.
	if m.mu.stopped {
		return errors.New("symdbManager has been stopped")
	}

	m.mu.queuedUploads = append(m.mu.queuedUploads, uploadRequest{
		runtimeID:      runtimeID,
		executablePath: executablePath,
	})
	// Signal the worker that a new request is available.
	m.mu.cond.Signal()
	return nil
}

// removeUpload removes any queued upload for the given process ID. If the
// upload is currently being processed, it will be cancelled and the call will
// block until the upload stops. If no upload for the respective process is in
// progress or queued, the call is a no-op.
func (m *symdbManager) removeUpload(runtimeID procRuntimeID) {
	m.mu.Lock()

	// Remove future uploads from queue.
	filtered := m.mu.queuedUploads[:0]
	for _, req := range m.mu.queuedUploads {
		if req.runtimeID.ProcessID != runtimeID.ProcessID {
			filtered = append(filtered, req)
		}
	}
	m.mu.queuedUploads = filtered

	// Deal with the case where the current upload is being removed: cancel and
	// wait for it to terminate.
	var doneCh chan struct{}
	if m.mu.currentUpload != nil && m.mu.currentUpload.runtimeID.ProcessID == runtimeID.ProcessID {
		log.Infof("Cancelling symbols upload for process %v", runtimeID.ProcessID)
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
		err := m.performUpload(uploadCtx, req.runtimeID, req.executablePath)
		if err != nil {
			if uploadCtx.Err() != nil {
				log.Infof("SymDB: upload cancelled for process %v (executable: %s): %v",
					req.runtimeID.ProcessID, req.executablePath, context.Cause(uploadCtx))
			} else {
				log.Errorf("SymDB: failed to upload symbols for process %v (executable: %s): %v",
					req.runtimeID.ProcessID, req.executablePath, err)
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

func (m *symdbManager) performUpload(ctx context.Context, runtimeID procRuntimeID, executablePath string) error {
	it, err := symdb.PackagesIterator(executablePath,
		symdb.ExtractOptions{
			Scope:                   symdb.ExtractScopeModulesFromSameOrg,
			IncludeInlinedFunctions: false,
		})
	if err != nil {
		return fmt.Errorf("failed to read symbols for process %v (executable: %s): %w",
			runtimeID.ProcessID, executablePath, err)
	}

	sender := uploader.NewSymDBUploader(
		m.uploadURL.String(),
		runtimeID.service, runtimeID.environment, runtimeID.version, runtimeID.runtimeID,
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
			log.Tracef("Uploading symbols chunk: %d packages, %d functions", len(uploadBuffer), bufferFuncs)
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
				runtimeID.ProcessID, executablePath, err)
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

	log.Infof("SymDB: Successfully uploaded symbols for process %v (executable: %s)",
		runtimeID.ProcessID, executablePath)
	return nil
}
