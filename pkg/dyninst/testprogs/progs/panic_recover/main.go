// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command panic_recover exercises the dyninst runtime.recovery probe: a
// probed function panics, an ancestor frame recovers, and dyninst should
// emit synthetic panic-unwound returns for every probed frame in the
// unwound region (instead of leaking the in-flight pairing state).
package main

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

func main() {
	// Wait for input before executing functions to allow time for uprobe attachment.
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()

	// Scenario 1: simple panic in C, recovered in A. A's frame returns
	// normally; B and C frames are unwound by panic.
	a()

	// Scenario 2: recover() is called but no panic in flight — the
	// recovery probe must not fire in this case.
	recoverNoPanic()

	// Scenario 3: a probed function that panics with a non-string value
	// (an error type) to exercise the interface-typed panic value.
	d()

	// Scenario 4: probed grandparent recovers; middle frame is unprobed;
	// probed grandchild panics. Validates that the unwound-region depth
	// bounds correctly include the probed grandchild and exclude the
	// probed grandparent, even though an unprobed frame sits between them.
	probedRecoverer()

	// Scenario 5: panic+recover entirely inside a goroutine with no
	// probed frames in flight on the main goroutine's stack at the time.
	// Validates the per-goid isolation: main goroutine's pairing state
	// (if any) must not be disturbed.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		goroutinePanic()
	}()
	wg.Wait()

	// Scenario 6: a panic+recover in a fresh goroutine where no probed
	// function is ever called. The BPF recovery handler should
	// short-circuit on the in_progress_calls lookup
	// (RecoveryNoOpenCalls counter) without reading the panic chain.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = recover() }()
		unprobedPanicker()
	}()
	wg.Wait()

	// Scenario 7: warm-up probe that captures an error-typed argument.
	// This exists so irgen pulls runtime.eface, the error interface,
	// and *errors.errorString into the type catalog; the recovery
	// probe's @exception chase can then traverse panic values whose
	// concrete type is *errors.errorString and render the inner
	// message instead of stopping at "missing type information".
	warmupErrorType(fmt.Errorf("warmup-message"))

	fmt.Println("done")
}

// warmupErrorType is probed (see panic_recover.yaml). Its only purpose
// is to keep an error-typed argument in the IR so irgen registers the
// error interface and the *errors.errorString concrete type with the
// type catalog. The recovery probe re-uses those registrations when it
// resolves the panic value's go runtime type pointer.
//
//go:noinline
func warmupErrorType(err error) {
	_ = err
}

//go:noinline
func a() {
	defer func() {
		_ = recover()
	}()
	b(42)
}

//go:noinline
func b(x int) string {
	return c(x, "hello")
}

//go:noinline
func c(x int, s string) string {
	if x == 42 {
		// fmt.Errorf with a format argument forces a heap allocation
		// (the formatted string ends up on the heap), so the panic
		// value's underlying bytes are guaranteed to be in resident
		// pages when the BPF recovery probe reads them. A string
		// literal would live in .rodata, whose pages may or may not
		// be paged in at recovery time, causing bpf_probe_read_user
		// to fail (with notCapturedReason=unavailable) on some Go
		// versions.
		panic(fmt.Errorf("synthetic from c with x=%d s=%q", x, s))
	}
	return s
}

//go:noinline
func recoverNoPanic() bool {
	r := recover()
	return r != nil
}

//go:noinline
func d() {
	defer func() {
		_ = recover()
	}()
	e()
}

//go:noinline
func e() {
	f()
}

//go:noinline
func f() {
	panic(fmt.Errorf("synthetic error from f"))
}

//go:noinline
func probedRecoverer() {
	defer func() {
		_ = recover()
	}()
	unprobedMiddle()
}

// unprobedMiddle has no probe attached; the recovery probe must skip
// over it correctly when computing the unwound region.
//
//go:noinline
func unprobedMiddle() {
	probedDeep()
}

// probedDeep conditionally panics, so the compiler emits a RET
// instruction (unlike functions that always panic — those have
// has_associated_return=false and never get a pairing slot). This
// ensures probedDeep's entry probe inserts into in_progress_calls
// and the recovery probe must evict its slot.
//
//go:noinline
func probedDeep() (out string) {
	if probedDeepShouldPanic {
		panic(fmt.Errorf("synthetic from probedDeep depth=%d",
			probedDeepDepth))
	}
	return "no panic"
}

// probedDeepDepth is read into the formatted panic value so the
// compiler can't fold the message into a literal. Set at init.
var probedDeepDepth = 42

// probedDeepShouldPanic is set to true at init; the compile-time
// constant elimination is defeated so the RET stays.
var probedDeepShouldPanic = true

//go:noinline
func goroutinePanic() {
	defer func() {
		_ = recover()
	}()
	goroutineInner()
}

// goroutineInner conditionally panics so it has a RET and gets a
// pairing slot inserted by its entry probe (see probedDeep comment).
//
//go:noinline
func goroutineInner() (out string) {
	if goroutineInnerShouldPanic {
		panic(fmt.Errorf("synthetic from goroutineInner gid=%d",
			goroutineInnerGid))
	}
	return "no panic"
}

var goroutineInnerShouldPanic = true
var goroutineInnerGid = 1

// unprobedPanicker has no probe attached. The recovery probe firing
// for its panic should hit the short-circuit path (no entries in
// in_progress_calls for the goroutine) and bump RecoveryNoOpenCalls.
//
//go:noinline
func unprobedPanicker() {
	panic(fmt.Errorf("synthetic from unprobedPanicker pid=%d",
		os.Getpid()))
}
