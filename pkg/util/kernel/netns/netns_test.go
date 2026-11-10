// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"errors"
	"os"
	"testing"

	"github.com/vishvananda/netns"
)

var errRestore = errors.New("forced restore failure")

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

func TestWithNSRestoreFailureJoinsErrors(t *testing.T) {
	calls := stubSetNS(t, errRestore)
	fnErr := errors.New("fn failed")
	err := WithNS(otherNS(t), func() error { return fnErr })
	if *calls != 2 {
		t.Fatalf("setNS called %d times, want 2", *calls)
	}
	if !errors.Is(err, errRestore) || !errors.Is(err, fnErr) {
		t.Fatalf("got %v, want both the fn and restore errors", err)
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
