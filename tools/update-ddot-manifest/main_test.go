// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package main

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// fixtureManifest exercises every classification branch: a core-stable module
// (v1.x), a core-beta module (v0.x), a contrib module (v0.x), a Datadog module
// and a third-party custom module.
const fixtureManifest = `dist:
  name: otelcol-contrib
  version: 0.100.0
exporters:
- gomod: go.opentelemetry.io/collector/exporter/debugexporter v0.100.0
providers:
- gomod: go.opentelemetry.io/collector/confmap/provider/envprovider v1.6.0
receivers:
- gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver v0.100.0
- gomod: github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl v0.100.0
- gomod: github.com/acme/customreceiver v1.2.3
replaces:
- github.com/foo => github.com/foo v0.0.1
`

var testVersion = ddotVersion{
	AgentVersion:      "9.9.9",
	OCBVersion:        "0.158.0",
	CoreStableVersion: "v1.64.0",
	ContribVersion:    "v0.158.0",
}

// gomods parses a manifest and returns a module->version map for all components.
func gomods(t *testing.T, data []byte) map[string]string {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parsing output: %v", err)
	}
	root := doc.Content[0]
	out := map[string]string{}
	for _, cat := range componentCategories {
		seq := mappingValue(root, cat)
		if seq == nil {
			continue
		}
		for _, item := range seq.Content {
			g := mappingValue(item, "gomod")
			if g == nil {
				continue
			}
			f := strings.Fields(g.Value)
			if len(f) == 2 {
				out[f[0]] = f[1]
			}
		}
	}
	if dist := mappingValue(root, "dist"); dist != nil {
		if ver := mappingValue(dist, "version"); ver != nil {
			out["dist.version"] = ver.Value
		}
	}
	return out
}

func TestUpdateManifest(t *testing.T) {
	out, res, err := updateManifest([]byte(fixtureManifest), testVersion)
	if err != nil {
		t.Fatalf("updateManifest: %v", err)
	}

	got := gomods(t, out)
	want := map[string]string{
		"go.opentelemetry.io/collector/exporter/debugexporter":                                  "v0.158.0", // core beta -> contrib stream
		"go.opentelemetry.io/collector/confmap/provider/envprovider":                            "v1.64.0",  // core stable -> v1.x stream
		"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver": "v0.158.0", // contrib
		"github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl":                   "v0.100.0", // datadog, untouched
		"github.com/acme/customreceiver":                                                        "v1.2.3",   // custom, untouched
		"dist.version":                                                                          "0.158.0",
	}
	for mod, wantVer := range want {
		if got[mod] != wantVer {
			t.Errorf("version for %s: got %q, want %q", mod, got[mod], wantVer)
		}
	}

	// Only the third-party module is reported as custom; the Datadog module is not.
	if len(res.custom) != 1 || res.custom[0] != "github.com/acme/customreceiver" {
		t.Errorf("custom components: got %v, want [github.com/acme/customreceiver]", res.custom)
	}

	// dist.version + 3 bumped components (debugexporter, envprovider, prometheusreceiver).
	if len(res.changed) != 4 {
		t.Errorf("changed count: got %d, want 4 (%v)", len(res.changed), res.changed)
	}

	// The replaces section must survive untouched.
	if !strings.Contains(string(out), "github.com/foo => github.com/foo v0.0.1") {
		t.Errorf("replaces section lost in output:\n%s", out)
	}
}

func TestUpdateManifestIdempotent(t *testing.T) {
	out1, _, err := updateManifest([]byte(fixtureManifest), testVersion)
	if err != nil {
		t.Fatalf("first update: %v", err)
	}
	out2, res2, err := updateManifest(out1, testVersion)
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if len(res2.changed) != 0 {
		t.Errorf("second run changed %d entries, want 0: %v", len(res2.changed), res2.changed)
	}
	if string(out1) != string(out2) {
		t.Errorf("output not stable across runs:\n--- first ---\n%s\n--- second ---\n%s", out1, out2)
	}
}

// TestUpdateManifestQuotedScalars checks that version values wrapped in YAML
// quotes (e.g. version: "0.100.0") are updated correctly. This was the bug
// reported in the PR review: the old regex could not match the quote between
// the whitespace and the version digit.
func TestUpdateManifestQuotedScalars(t *testing.T) {
	quotedManifest := `dist:
  name: otelcol-contrib
  version: "0.100.0"
exporters:
- gomod: go.opentelemetry.io/collector/exporter/debugexporter v0.100.0
`
	out, _, err := updateManifest([]byte(quotedManifest), testVersion)
	if err != nil {
		t.Fatalf("updateManifest: %v", err)
	}
	got := gomods(t, out)
	if got["dist.version"] != "0.158.0" {
		t.Errorf("dist.version: got %q, want %q", got["dist.version"], "0.158.0")
	}
}

func TestLookupVersionUnknown(t *testing.T) {
	if _, err := lookupVersion("0.0.0"); err == nil {
		t.Error("expected error for unknown agent version, got nil")
	}
	if _, err := lookupVersion("7.83.2"); err != nil {
		t.Errorf("expected known version 7.83.2 to resolve, got %v", err)
	}
}
