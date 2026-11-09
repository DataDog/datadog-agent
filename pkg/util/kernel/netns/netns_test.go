// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

var errRestore = errors.New("forced restore failure")

// Pin the main goroutine to the main thread, which the runtime never exits,
// so test goroutines always run on threads that can be discarded.
func init() {
	runtime.LockOSThread()
}

// otherNS returns a handle that never equals the current namespace, so WithNS
// takes the switching path. setNS is stubbed, so it is never passed to setns.
func otherNS(t *testing.T) netns.NsHandle {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return netns.NsHandle(f.Fd())
}

// stubSetNS makes every setNS call after the first fail with failAfter.
func stubSetNS(t *testing.T, failAfter error) *int {
	orig := setNS
	t.Cleanup(func() { setNS = orig })
	calls := 0
	setNS = func(netns.NsHandle) error {
		calls++
		if calls > 1 {
			return failAfter
		}
		return nil
	}
	return &calls
}

// runOnGoroutine runs WithNS on a fresh goroutine and returns its error and
// the OS thread it ran on once that goroutine has exited.
func runOnGoroutine(ns netns.NsHandle, fn func() error) (tid int, err error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		err = WithNS(ns, func() error {
			tid = unix.Gettid()
			return fn()
		})
	}()
	<-done
	return tid, err
}

func threadExists(tid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/self/task/%d", tid))
	return err == nil
}

func TestWithNSRestoreFailureDiscardsThread(t *testing.T) {
	calls := stubSetNS(t, errRestore)
	fnErr := errors.New("fn failed")
	tid, err := runOnGoroutine(otherNS(t), func() error { return fnErr })
	if *calls != 2 {
		t.Fatalf("setNS called %d times, want 2", *calls)
	}
	if !errors.Is(err, errRestore) || !errors.Is(err, fnErr) {
		t.Fatalf("got %v, want both the fn and restore errors", err)
	}
	// A goroutine that exits while locked takes its thread with it.
	deadline := time.Now().Add(5 * time.Second)
	for threadExists(tid) {
		if time.Now().After(deadline) {
			t.Fatalf("thread %d still exists; it was returned to the scheduler", tid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWithNSRestoreSuccessKeepsThread(t *testing.T) {
	stubSetNS(t, nil)
	tid, err := runOnGoroutine(otherNS(t), func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if !threadExists(tid) {
		t.Fatalf("thread %d exited; it should have been unlocked and reused", tid)
	}
}

func TestWithNSRestoresOnPanic(t *testing.T) {
	calls := stubSetNS(t, nil)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected the panic to propagate")
			}
		}()
		_ = WithNS(otherNS(t), func() error { panic("boom") })
	}()
	if *calls != 2 {
		t.Fatalf("setNS called %d times, want 2 (switch and restore)", *calls)
	}
}
