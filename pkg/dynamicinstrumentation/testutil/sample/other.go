// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sample

import (
	"bytes"
	"runtime"
	"strconv"
)

type triggerVerifierErrorForTesting byte

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
func ExecuteOther() {
	x := make(chan bool)
	test_channel(x)

	test_trigger_verifier_error(1)
}
