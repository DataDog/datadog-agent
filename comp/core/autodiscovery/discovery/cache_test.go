// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	"testing"
	"time"
)

func TestProbeCache_HitAndExpiry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	clock := func() time.Time { return now }
	c := newProbeCache(clock)

	// Empty cache — miss.
	if _, _, ok := c.get("svc1", "h1"); ok {
		t.Fatal("expected miss on empty cache")
	}

	// Successful probe entry, never expires.
	c.putSuccess("svc1", "h1", ProbeResult{Port: 8090})
	if r, success, ok := c.get("svc1", "h1"); !ok || !success || r.Port != 8090 {
		t.Fatalf("expected hit success(8090); got ok=%v success=%v port=%d", ok, success, r.Port)
	}

	// Failed probe entry, expires after 30s.
	c.putFailure("svc1", "h2", 30*time.Second)
	if _, success, ok := c.get("svc1", "h2"); !ok || success {
		t.Fatal("expected hit failure")
	}
	now = now.Add(31 * time.Second)
	if _, _, ok := c.get("svc1", "h2"); ok {
		t.Fatal("expected miss after expiry")
	}
}

func TestProbeCache_DifferentKeysIsolated(t *testing.T) {
	now := time.Unix(0, 0)
	c := newProbeCache(func() time.Time { return now })
	c.putSuccess("svcA", "h1", ProbeResult{Port: 1})
	c.putSuccess("svcB", "h1", ProbeResult{Port: 2})
	if r, _, _ := c.get("svcA", "h1"); r.Port != 1 {
		t.Fatalf("svcA: got %d", r.Port)
	}
	if r, _, _ := c.get("svcB", "h1"); r.Port != 2 {
		t.Fatalf("svcB: got %d", r.Port)
	}
}
