// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package configingestion subscribes to workloadmeta Process events and ships
// discovered config files to the EvP intake.
package configingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxFileBytes       = 256 * 1024
	configSourceNative = "app_native"
)

// Config holds the runtime configuration for the watcher.
type Config struct {
	IntakeURL string
	APIKey    string
	HostID    string
}

// shipTask collects the data needed to ship one config file outside the lock.
type shipTask struct {
	pid         int32
	path        string
	integration string
}

// Watcher subscribes to workloadmeta Process events and ships discovered config files to EvP.
type Watcher struct {
	store  workloadmeta.Component
	cfg    Config
	client *http.Client

	// seenFiles is keyed by file path; dedup is permanent for the demo lifetime.
	seenFiles map[string]struct{}
	// pidFiles maps PID → shipped paths for cleanup on EventTypeUnset.
	pidFiles map[int32][]string
	mu       sync.Mutex

	// cancel is initialized to a no-op in NewWatcher and replaced under mu in Start.
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once // ensures Start is called at most once
}

// NewWatcher returns a new Watcher ready to Start.
func NewWatcher(store workloadmeta.Component, cfg Config) *Watcher {
	return &Watcher{
		store:     store,
		cfg:       cfg,
		client:    &http.Client{Timeout: 10 * time.Second},
		seenFiles: make(map[string]struct{}),
		pidFiles:  make(map[int32][]string),
		cancel:    func() {}, // no-op until Start sets the real cancel
	}
}

// Start subscribes to workloadmeta and processes events in a background goroutine.
// The goroutine runs until Stop is called. Calling Start more than once panics.
//
// TODO(DSCVR Phase D): change the return type to error and return instead of
// panicking, so Fx OnStart hooks can surface the failure cleanly rather than
// producing an opaque stack trace through the Fx startup machinery.
func (w *Watcher) Start(ctx context.Context) {
	called := false
	w.startOnce.Do(func() { called = true })
	if !called {
		panic("configingestion.Watcher.Start called more than once")
	}

	ctx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancel = cancel
	w.mu.Unlock()

	filter := workloadmeta.NewFilterBuilder().
		AddKindWithEntityFilter(workloadmeta.KindProcess, func(e workloadmeta.Entity) bool {
			p, ok := e.(*workloadmeta.Process)
			return ok && p.Service != nil && p.ContainerID == "" && len(p.Service.ConfigFiles) > 0
		}).
		Build()

	ch := w.store.Subscribe("config-ingestion", workloadmeta.NormalPriority, filter)

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer w.store.Unsubscribe(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case bundle, ok := <-ch:
				if !ok {
					return
				}
				bundle.Acknowledge()
				w.handleBundle(ctx, bundle)
			}
		}
	}()
}

// Stop cancels the background goroutine and waits for it to exit before returning.
// This ensures workloadmeta resources are not accessed after Stop returns.
func (w *Watcher) Stop() {
	w.mu.Lock()
	w.cancel()
	w.mu.Unlock()
	w.wg.Wait()
}

func (w *Watcher) handleBundle(ctx context.Context, bundle workloadmeta.EventBundle) {
	// Collect work under the lock, then perform I/O outside to avoid holding
	// the mutex during potentially slow HTTP calls.
	w.mu.Lock()
	var toShip []shipTask
	// pending guards against shipping the same path twice when a single bundle
	// contains multiple EventTypeSet events referencing the same file.
	pending := make(map[string]struct{})
	for _, ev := range bundle.Events {
		process, ok := ev.Entity.(*workloadmeta.Process)
		if !ok {
			continue
		}
		switch ev.Type {
		case workloadmeta.EventTypeSet:
			if process.Service == nil {
				continue
			}
			integration := integrationFromService(process.Service)
			for _, path := range process.Service.ConfigFiles {
				if _, alreadySeen := w.seenFiles[path]; alreadySeen {
					continue
				}
				if _, alreadyPending := pending[path]; alreadyPending {
					continue
				}
				pending[path] = struct{}{}
				toShip = append(toShip, shipTask{pid: process.Pid, path: path, integration: integration})
			}
		case workloadmeta.EventTypeUnset:
			// Don't remove from seenFiles — dedup is permanent for the demo.
			delete(w.pidFiles, process.Pid)
		}
	}
	w.mu.Unlock()

	// Note: handleBundle is called from a single goroutine (the subscriber loop
	// in Start), so the window between the dedup check above and the seenFiles
	// update below is not a TOCTOU race in practice. Any future change to
	// parallelize shipping must re-evaluate this invariant.
	for _, task := range toShip {
		if err := w.ship(ctx, task.path, task.integration); err != nil {
			log.Warnf("configingestion: failed to ship %s: %v", task.path, err)
			continue
		}
		log.Infof("configingestion: shipped %s (integration=%s)", task.path, task.integration)
		w.mu.Lock()
		w.seenFiles[task.path] = struct{}{}
		w.pidFiles[task.pid] = append(w.pidFiles[task.pid], task.path)
		w.mu.Unlock()
	}
}

func (w *Watcher) ship(ctx context.Context, path, integration string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if len(raw) > maxFileBytes {
		return fmt.Errorf("file exceeds %d KiB limit", maxFileBytes/1024)
	}

	// Validate the full content — config files must be UTF-8 text.
	if !utf8.Valid(raw) {
		return errors.New("file does not appear to be UTF-8 text")
	}

	content := redactSensitive(string(raw))
	ct := detectContentType(integration, path)
	env := buildEnvelope(w.cfg.HostID, integration, configSourceNative, path, ct, content)

	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	return postEnvelope(ctx, w.client, w.cfg.IntakeURL, w.cfg.APIKey, body)
}
