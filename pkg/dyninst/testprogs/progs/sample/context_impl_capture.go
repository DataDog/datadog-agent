// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// contextImpl is a struct that implements context.Context but has no
// context.Context field of its own, so it is not a link in a walkable context
// chain (for example rapid.Context, whose context methods forward to the
// request's context). It is captured as a plain struct argument, so the
// debugger must capture its fields normally rather than treating it as a
// context to chain-walk. Each interface field below is backed by a concrete
// type from a different package (time, bytes, strings, errors); resolving those
// cross-package concrete types is what regressed to "UnknownType(...) out of
// bounds" when such a struct was wrongly handled as a context.
type contextImpl struct {
	logger    fmt.Stringer // concrete type: time.Duration
	responder io.Writer    // concrete type: *bytes.Buffer
	decoder   io.Reader    // concrete type: *strings.Reader
	params    error        // concrete type: *errors.errorString (unexported)
	secrets   fmt.Stringer // concrete type: time.Time
}

var _ context.Context = (*contextImpl)(nil)

func (c *contextImpl) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *contextImpl) Done() <-chan struct{}       { return nil }
func (c *contextImpl) Err() error                  { return nil }
func (c *contextImpl) Value(any) any               { return nil }

//nolint:all
//go:noinline
func testCaptureContextImpl(c *contextImpl) {}

//nolint:all
//go:noinline
func useAsContext(ctx context.Context) error { return ctx.Err() }

func newContextImpl() *contextImpl {
	return &contextImpl{
		logger:    5 * time.Second,
		responder: bytes.NewBufferString("response body"),
		decoder:   strings.NewReader("request body"),
		params:    errors.New("param error"),
		secrets:   time.Unix(1700000000, 0).UTC(),
	}
}

//nolint:all
func executeContextImplFuncs() {
	c := newContextImpl()
	// Ensure the *contextImpl -> context.Context itab is emitted so the type is
	// discovered as a context implementation, which is what triggered the bug.
	_ = useAsContext(c)
	testCaptureContextImpl(c)
}
