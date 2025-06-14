// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sample

import (
	"bytes"
	"io"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type triggerVerifierErrorForTesting byte

type largeType struct {
	mu             sync.RWMutex
	output         []byte
	w              io.Writer
	ran            bool
	failed         bool
	skipped        bool
	done           bool
	helperPCs      map[uintptr]struct{}
	helperNames    map[string]struct{}
	cleanups       []func()
	cleanupName    string
	cleanupPc      []uintptr
	finished       bool
	inFuzzFn       bool
	chatty         interface{}
	bench          bool
	hasSub         atomic.Bool
	cleanupStarted atomic.Bool
	runner         string
	isParallel     bool
	parent         *largeType
	level          int
	creator        []uintptr
	name           string
	start          time.Time
}

//nolint:all
//go:noinline
func test_channel(c chan bool) {}

//nolint:all
//go:noinline
func test_trigger_verifier_error(t triggerVerifierErrorForTesting) {}

// return_goroutine_id gets the goroutine ID and returns it
//
//nolint:all
//go:noinline
func Return_goroutine_id() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

//nolint:all
//go:noinline
func accept_large_type(t *largeType) {}

//nolint:all
//go:noinline
func ExecuteOther() {
	x := make(chan bool)
	test_channel(x)

	test_trigger_verifier_error(1)

	accept_large_type(&largeType{
		helperPCs:   make(map[uintptr]struct{}),
		helperNames: make(map[string]struct{}),
		start:       time.Now(),
	})
}
