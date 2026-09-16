// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloads

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
)

// The workloads top-level section is a list; each item decodes through the
// per-entry Schema. These tests exercise a single item.
func TestEntryRoundTrip(t *testing.T) {
	w, _, err := Schema.Decode([]byte("app: nginx"), "workloads[0]")
	if err != nil {
		t.Fatal(err)
	}
	if w.App != "nginx" {
		t.Fatalf("unexpected round-trip: %+v", w)
	}

	w, _, err = Schema.Decode([]byte("image: ghcr.io/datadog/redis:latest\nname: my-redis"), "workloads[1]")
	if err != nil {
		t.Fatal(err)
	}
	if w.Image != "ghcr.io/datadog/redis:latest" || w.Name != "my-redis" {
		t.Fatalf("unexpected round-trip: %+v", w)
	}
}

func TestMutualExclusion(t *testing.T) {
	for name, raw := range map[string]string{
		"none set":      "name: orphan",
		"both set":      "app: nginx\nimage: foo",
		"all three":     "app: nginx\nimage: foo\nmanifest: bar",
		"unknown field": "app: nginx\nflavor: spicy",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Schema.Decode([]byte(raw), "workloads[0]"); err == nil {
				t.Fatalf("expected a validation error for %q", raw)
			}
		})
	}
}

func TestEmptyEntryIsRejected(t *testing.T) {
	// An empty mapping fails the exactly-one-form rule.
	if _, _, err := Schema.Decode(nil, "workloads[0]"); err == nil {
		t.Fatal("expected mutual-exclusion error for an empty workload entry")
	}
}

func TestExampleIsValid(t *testing.T) {
	node, err := Schema.Example(nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := configschema.Encode(node)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "api-key") {
		t.Fatal("example must not contain credentials")
	}
}
