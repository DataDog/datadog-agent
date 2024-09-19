package main

import (
	"bytes"
	"runtime"
	"strconv"
)

type triggerVerifierErrorForTesting byte

//go:noinline
func test_trigger_verifier_error(t triggerVerifierErrorForTesting) {}

// return_goroutine_id gets the goroutine ID and returns it
//
//go:noinline
func return_goroutine_id() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

//go:noinline
func executeOther() {
	test_trigger_verifier_error(1)
}
