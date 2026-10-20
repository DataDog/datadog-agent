// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
)

// contextFile wraps an io.ReadWriteCloser and ensures Close is called when
// the associated context expires. This acts as a safety backstop to prevent
// file descriptor leaks if a caller omits an explicit Close call. Callers
// that already defer Close (the common case) are unaffected — the underlying
// file is closed at most once regardless of how many times Close is invoked.
type contextFile struct {
	io.ReadWriteCloser
	closeOnce sync.Once
	stopOnce  sync.Once
	stopped   chan struct{}
}

// WithContextClose wraps f so that f.Close() is guaranteed to be called when
// ctx is done, in addition to any explicit Close calls made by the caller.
// The underlying file is closed at most once.
//
// A background goroutine waits for either ctx cancellation or an explicit
// Close call. If the context expires first the file is closed immediately and
// the goroutine exits. If Close is called first the goroutine is signalled to
// stop and exits without touching the file again.
func WithContextClose(ctx context.Context, f io.ReadWriteCloser) io.ReadWriteCloser {
	cf := &contextFile{
		ReadWriteCloser: f,
		stopped:         make(chan struct{}),
	}
	go func() {
		select {
		case <-ctx.Done():
			cf.closeOnce.Do(func() {
				cf.ReadWriteCloser.Close() //nolint:errcheck
			})
		case <-cf.stopped:
			// explicitly closed; goroutine exits cleanly
		}
	}()
	return cf
}

// Stat forwards to the wrapped ReadWriteCloser's Stat method when it has
// one, so callers that type-assert for Stat (e.g. sed -i's identity-pinning
// read, or the isRegularFile FIFO/regular-file check) see through this
// wrapper to the real file's metadata rather than losing that capability
// merely because the value passed through WithContextClose. Embedding
// io.ReadWriteCloser (an interface, which has no Stat method) does not
// promote a concrete Stat method the way embedding *os.File directly would,
// so this must be forwarded explicitly.
func (cf *contextFile) Stat() (os.FileInfo, error) {
	type statCloser interface{ Stat() (os.FileInfo, error) }
	sc, ok := cf.ReadWriteCloser.(statCloser)
	if !ok {
		return nil, errors.New("file does not support Stat")
	}
	return sc.Stat()
}

// Close closes the underlying file and signals the background goroutine to
// exit. Safe to call multiple times; the file is closed at most once.
func (cf *contextFile) Close() error {
	// Signal the goroutine to exit before closing, so it cannot race to
	// close the file after we return.
	cf.stopOnce.Do(func() {
		close(cf.stopped)
	})
	var err error
	cf.closeOnce.Do(func() {
		err = cf.ReadWriteCloser.Close()
	})
	return err
}
