// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import "testing"

func TestDecodeAppliesDefaultsAndValidates(t *testing.T) {
	var got schemaSample
	s, _ := BuildSchema(&schemaSample{})

	// token is required; supply it. os omitted -> default. verbose true.
	err := Decode(s, map[string]string{"token": "abc", "verbose": "true", "replicas": "3"}, &got)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.OS != "ubuntu-22.04" {
		t.Errorf("default not applied, OS=%q", got.OS)
	}
	if got.Required != "abc" || !got.Verbose || got.Replicas != 3 {
		t.Errorf("decoded wrong: %+v", got)
	}
}

func TestDecodeErrors(t *testing.T) {
	s, _ := BuildSchema(&schemaSample{})
	cases := map[string]map[string]string{
		"missing required": {"os": "debian-12"},
		"bad enum":         {"token": "x", "os": "windows"},
		"unknown key":      {"token": "x", "nope": "1"},
		"bad int":          {"token": "x", "replicas": "abc"},
	}
	for name, vals := range cases {
		var dst schemaSample
		if err := Decode(s, vals, &dst); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestDecodeOverlaysDefaults(t *testing.T) {
	var got defaultsSample
	s, _ := BuildSchema(&got)
	// Only "os" provided; count/enabled must come from defaults.
	if err := Decode(s, map[string]string{"os": "debian-12"}, &got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.OS != "debian-12" || got.Count != 3 || !got.Enabled {
		t.Fatalf("overlay wrong: %+v", got)
	}
}

func TestDecodeOverlayOnAlreadyDefaulted(t *testing.T) {
	got := NewParams[defaultsSample]() // already defaulted
	s, _ := BuildSchema(got)
	if err := Decode(s, map[string]string{"enabled": "false"}, got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.OS != "ubuntu-22.04" || got.Count != 3 || got.Enabled {
		t.Fatalf("expected only enabled overridden: %+v", got)
	}
}
