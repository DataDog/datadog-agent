// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package cache

import "testing"

func TestKeyCache(t *testing.T) {
	kc := newKeyCache(3)
	var (
		got EvictReason
		ok  bool
	)
	expect := func(wantSize int, want EvictReason, wantOK bool) {
		if got != want {
			t.Fatalf("reason %s != %s", got, want)
		}
		if ok != wantOK {
			t.Fatalf("seen %v != %v", ok, wantOK)
		}
		if wantSize != kc.len() {
			t.Fatalf("size %d != %d", kc.len(), wantSize)
		}
	}

	got, ok = kc.Mark(1, EvictReasonRoot)
	// first item, size is one
	expect(1, EvictReasonRoot, false)

	got, ok = kc.Mark(2, EvictReasonSpace)
	// second item, size grows to two
	expect(2, EvictReasonSpace, false)

	got, ok = kc.Mark(3, EvictReasonIdle)
	// third item, size grows to three
	expect(3, EvictReasonIdle, false)

	got, ok = kc.Mark(4, EvictReasonStopping)
	// overflows: oldest (1) gets evicted, 3 remain
	expect(3, EvictReasonStopping, false)

	got, ok = kc.Mark(3, EvictReasonStopping)
	// key 3 was seen before for reason:idle
	expect(3, EvictReasonIdle, true)

	got, ok = kc.Mark(2, EvictReasonIdle)
	// key 2 was seen before for reason:space
	expect(3, EvictReasonSpace, true)

	got, ok = kc.Mark(1, EvictReasonIdle)
	// key 1 was evicted so it is now new from the key cache's perspective
	expect(3, EvictReasonIdle, false)
}
