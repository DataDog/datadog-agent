// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import "testing"

func TestScopeFromTagsUsesExplicitValuesAndFirstTagFallback(t *testing.T) {
	tags := []string{"service:first", "source:nginx", "service:second", "source:apache"}
	if got, want := scopeFromTags(tags, "", ""), (scopeKey{service: "first", source: "nginx"}); got != want {
		t.Fatalf("scopeFromTags() = %#v, want %#v", got, want)
	}
	if got, want := scopeFromTags(tags, "resolved-service", "resolved-source"), (scopeKey{service: "resolved-service", source: "resolved-source"}); got != want {
		t.Fatalf("scopeFromTags() with resolved values = %#v, want %#v", got, want)
	}
}

func TestNormalizeScopeTagsKeepsResolvedScopeStable(t *testing.T) {
	scope := scopeKey{service: "api", source: "nginx"}
	tags := normalizeScopeTags([]string{"env:prod", "service:other", "source:other", "service:again"}, scope)
	if got, want := scopeFromTags(tags, "", ""), scope; got != want {
		t.Fatalf("normalized scope = %#v, want %#v (tags: %#v)", got, want, tags)
	}
	if got, want := len(tags), 3; got != want {
		t.Fatalf("normalized tag count = %d, want %d", got, want)
	}
}

func TestScopeRegistryBoundsAndRetainsTailers(t *testing.T) {
	r := newScopeRegistry(2, nil)
	a := scopeKey{service: "api", source: "nginx"}
	b := scopeKey{service: "worker", source: "app"}
	c := scopeKey{service: "db", source: "postgres"}
	if !r.admit(a, "metric_input") || !r.markTailerStarted(b) {
		t.Fatal("expected first two scopes to be admitted")
	}
	if !r.hasTailer(b) || r.hasTailer(a) {
		t.Fatal("tailer membership was not retained correctly")
	}
	if r.admit(c, "anomaly") {
		t.Fatal("expected third scope to be rejected at capacity")
	}
	if !r.admit(a, "log_input") {
		t.Fatal("expected an already admitted scope to remain admitted")
	}
}

func TestScopeTelemetryTagsAndDefaultLimit(t *testing.T) {
	service, source := (scopeKey{}).telemetryTags()
	if service != "none" || source != "none" {
		t.Fatalf("empty scope telemetry tags = (%q, %q), want (none, none)", service, source)
	}
	if got := scorerScopeLimit(nil); got != defaultMaxScopes {
		t.Fatalf("scorerScopeLimit(nil) = %d, want %d", got, defaultMaxScopes)
	}
}
