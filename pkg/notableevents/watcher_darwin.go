// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

/*
#cgo LDFLAGS: -framework CoreServices

#include <stdlib.h>
#include "watcher_fsevents_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/cgo"
	"sort"
	"strings"
	"sync"
	"unsafe"
)

type darwinReportWatcher interface {
	Events() <-chan string
	Errors() <-chan error
	Update(paths []string) error
	Close() error
}

type darwinReportWatcherFactory func() (darwinReportWatcher, error)

type darwinDirectoryWatcher struct {
	events chan string
	errors chan error
	done   chan struct{}
	wake   chan struct{}

	updateMu sync.Mutex
	stream   unsafe.Pointer
	handle   cgo.Handle
	closed   bool

	watchedMu sync.RWMutex
	watched   map[string]struct{}

	pendingMu sync.Mutex
	pending   map[string]struct{}

	pumpWG sync.WaitGroup
}

const fseventsDroppedFlags = uint32(0x1 | 0x2 | 0x4)

// newDarwinReportWatcher creates an FSEvents watcher with coalesced directory notifications.
func newDarwinReportWatcher() (darwinReportWatcher, error) {
	watcher := &darwinDirectoryWatcher{
		events:  make(chan string, 32),
		errors:  make(chan error, 8),
		watched: make(map[string]struct{}),
		pending: make(map[string]struct{}),
		done:    make(chan struct{}),
		wake:    make(chan struct{}, 1),
	}
	watcher.handle = cgo.NewHandle(watcher)
	watcher.pumpWG.Add(1)
	go watcher.pumpEvents()
	return watcher, nil
}

// Events returns the stream of diagnostic-report directories requiring reconciliation.
func (w *darwinDirectoryWatcher) Events() <-chan string {
	return w.events
}

// Errors returns asynchronous failures reported by the FSEvents stream.
func (w *darwinDirectoryWatcher) Errors() <-chan error {
	return w.errors
}

// Update replaces the watched directory set and restarts the stream when necessary.
func (w *darwinDirectoryWatcher) Update(paths []string) error {
	desired := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		desired[filepath.Clean(path)] = struct{}{}
	}

	w.updateMu.Lock()
	defer w.updateMu.Unlock()
	if w.closed {
		return errors.New("FSEvents watcher is closed")
	}

	w.watchedMu.RLock()
	unchanged := samePathSet(w.watched, desired)
	w.watchedMu.RUnlock()
	if unchanged && (len(desired) == 0 || w.stream != nil) {
		return nil
	}

	w.stopStream()
	w.watchedMu.Lock()
	w.watched = desired
	w.watchedMu.Unlock()
	if len(desired) == 0 {
		return nil
	}

	sortedPaths := make([]string, 0, len(desired))
	for path := range desired {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)
	return w.startStream(sortedPaths)
}

// Close stops FSEvents delivery and releases the watcher's goroutine and handle.
func (w *darwinDirectoryWatcher) Close() error {
	w.updateMu.Lock()
	if w.closed {
		w.updateMu.Unlock()
		return nil
	}
	w.closed = true
	w.stopStream()
	w.updateMu.Unlock()

	close(w.done)
	w.pumpWG.Wait()
	w.handle.Delete()
	close(w.events)
	close(w.errors)
	return nil
}

// pumpEvents drains coalesced notifications into the public event channel.
func (w *darwinDirectoryWatcher) pumpEvents() {
	defer w.pumpWG.Done()
	for {
		select {
		case <-w.done:
			return
		case <-w.wake:
			for {
				path, ok := w.popPending()
				if !ok {
					break
				}
				select {
				case w.events <- path:
				case <-w.done:
					return
				}
			}
		}
	}
}

// enqueue records a directory once and wakes the event pump.
func (w *darwinDirectoryWatcher) enqueue(path string) {
	w.pendingMu.Lock()
	w.pending[path] = struct{}{}
	w.pendingMu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// popPending removes one coalesced directory notification for delivery.
func (w *darwinDirectoryWatcher) popPending() (string, bool) {
	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()
	for path := range w.pending {
		delete(w.pending, path)
		return path, true
	}
	return "", false
}

// watchedDirectoryForPath maps a changed descendant path to its configured watch root.
func (w *darwinDirectoryWatcher) watchedDirectoryForPath(path string) (string, bool) {
	cleanPath := filepath.Clean(path)

	w.watchedMu.RLock()
	defer w.watchedMu.RUnlock()
	for watchedPath := range w.watched {
		if cleanPath == watchedPath || strings.HasPrefix(cleanPath, watchedPath+string(os.PathSeparator)) {
			return watchedPath, true
		}
	}
	return "", false
}

// enqueueAllWatched schedules full reconciliation after dropped FSEvents.
func (w *darwinDirectoryWatcher) enqueueAllWatched() {
	w.watchedMu.RLock()
	defer w.watchedMu.RUnlock()
	for path := range w.watched {
		w.enqueue(path)
	}
}

// reportError publishes an asynchronous watcher error without blocking callbacks.
func (w *darwinDirectoryWatcher) reportError(err error) {
	select {
	case w.errors <- err:
	default:
	}
}

// startStream creates and starts the native FSEvents stream for the requested paths.
func (w *darwinDirectoryWatcher) startStream(paths []string) error {
	cPaths := make([]*C.char, len(paths))
	for index, path := range paths {
		cPaths[index] = C.CString(path)
		defer C.free(unsafe.Pointer(cPaths[index]))
	}

	var errorMessage *C.char
	stream := C.dd_pkg_notableevents_fsevents_start(
		(**C.char)(unsafe.Pointer(&cPaths[0])),
		C.size_t(len(cPaths)),
		C.uintptr_t(w.handle),
		&errorMessage,
	)
	if stream == nil {
		if errorMessage == nil {
			return errors.New("failed to start FSEvents watcher")
		}
		defer C.free(unsafe.Pointer(errorMessage))
		return fmt.Errorf("failed to start FSEvents watcher: %s", C.GoString(errorMessage))
	}
	w.stream = stream
	return nil
}

// stopStream shuts down and invalidates the active native stream.
func (w *darwinDirectoryWatcher) stopStream() {
	if w.stream == nil {
		return
	}
	C.dd_pkg_notableevents_fsevents_stop(w.stream)
	w.stream = nil
}

// samePathSet reports whether two normalized watch sets contain identical paths.
func samePathSet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for path := range left {
		if _, found := right[path]; !found {
			return false
		}
	}
	return true
}

// goPkgNotableEventsFSEventsCallback translates native FSEvents callbacks into directory reconciliations.
//
//export goPkgNotableEventsFSEventsCallback
func goPkgNotableEventsFSEventsCallback(rawHandle C.uintptr_t, rawPath *C.char, rawFlags C.uint32_t) {
	watcher, ok := cgo.Handle(rawHandle).Value().(*darwinDirectoryWatcher)
	if !ok {
		return
	}

	flags := uint32(rawFlags)
	if flags&fseventsDroppedFlags != 0 {
		watcher.reportError(fmt.Errorf("FSEvents dropped events (flags %#x); scheduling full watched-directory rescan", flags))
		watcher.enqueueAllWatched()
		return
	}

	if watchedDir, found := watcher.watchedDirectoryForPath(C.GoString(rawPath)); found {
		watcher.enqueue(watchedDir)
	}
}
