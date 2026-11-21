// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build python

package python

import (
	"context"
	"errors"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

/*
#include <stdlib.h>
#include "datadog_agent_rtloader.h"
*/
import "C"

// SupportsRemoteQueries is true exactly when the instance has a callable remote_query.
func (c *PythonCheck) SupportsRemoteQueries() bool {
	lock, err := newStickyLock()
	if err != nil {
		return false
	}
	defer lock.unlock()
	return C.has_remote_query(rtloader, c.instance) != 0
}

// RemoteQuery calls the instance independently of its scheduled runs. The sticky
// lock pins this goroutine to its OS thread, not to the check. Python releases
// the GIL during database/network waits; no Go-side lock covers the call.
func (c *PythonCheck) RemoteQuery(ctx context.Context, request string) (string, error) {
	lock, err := newStickyLock()
	if err != nil {
		return "", errors.New("Python runtime is unavailable")
	}
	defer lock.unlock()
	// Check under the GIL immediately before entering Python. The core repeats
	// the idempotent cancellation signal until the call ends, since Python can
	// also switch threads before the integration registers its run.
	if c.cancelled.Load() {
		return "", check.ErrRemoteQueryCancelled
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	input := C.CString(request)
	defer C.free(unsafe.Pointer(input))
	result := C.remote_query(rtloader, c.instance, input)
	if result == nil {
		_ = getRtLoaderError()
		return "", errors.New("remote_query did not return a string outcome")
	}
	defer C.rtloader_free(rtloader, unsafe.Pointer(result))
	return C.GoString(result), nil
}

// CancelRemoteQuery signals all of this check's active runs with this crawl ID.
func (c *PythonCheck) CancelRemoteQuery(crawlID string) error {
	lock, err := newStickyLock()
	if err != nil {
		return errors.New("Python runtime is unavailable")
	}
	defer lock.unlock()
	if c.cancelled.Load() {
		return nil
	}
	id := C.CString(crawlID)
	defer C.free(unsafe.Pointer(id))
	if C.cancel_remote_query(rtloader, c.instance, id) == 0 {
		_ = getRtLoaderError()
		return errors.New("cancel_remote_query call failed")
	}
	return nil
}
