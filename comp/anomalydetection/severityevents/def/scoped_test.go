// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package severityevents

import (
	"hash/fnv"
	"testing"
)

func TestScopeKeyForService_EmptyIsZero(t *testing.T) {
	if got := ScopeKeyForService(""); got != 0 {
		t.Fatalf("expected empty service key 0, got %d", got)
	}
}

func TestScopeKeyForService_MatchesFNV64aAndDistinct(t *testing.T) {
	services := []string{"checkout", "payments", "auth-service", "web"}
	seen := make(map[ScopeKey]string, len(services))

	for _, service := range services {
		hasher := fnv.New64a()
		_, _ = hasher.Write([]byte(service))
		want := ScopeKey(hasher.Sum64())
		if want == 0 {
			want = 1
		}

		got := ScopeKeyForService(service)
		if got != want {
			t.Fatalf("ScopeKeyForService(%q) = %d, want %d", service, got, want)
		}
		if prev, duplicate := seen[got]; duplicate {
			t.Fatalf("unexpected key collision between %q and %q", prev, service)
		}
		seen[got] = service
	}
}

func TestScopeKeyForService_ZeroAllocs(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		if ScopeKeyForService("checkout-service") == 0 {
			t.Fatal("unexpected zero key")
		}
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocations, got %v", allocs)
	}
}
