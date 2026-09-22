// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localpackage

import (
	"testing"
)

func TestSectionRoundTrip(t *testing.T) {
	c, _, err := Schema.Decode([]byte("allow-unsigned: true\nsha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nconfig: |\n  log_level: debug\n"), "agent.package")
	if err != nil {
		t.Fatal(err)
	}
	if !c.AllowUnsigned || c.SHA256 != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected section: %+v", c)
	}
}

func TestSectionRules(t *testing.T) {
	// The derived pipeline shape: no sha256 — the download provider pins the
	// downloaded file in its receipt instead of a user-typed digest.
	if _, _, err := Schema.Decode([]byte("allow-unsigned: true\n"), "agent.package"); err != nil {
		t.Fatalf("derived pipeline section must decode without a user-typed digest: %v", err)
	}
	for name, raw := range map[string]string{
		"short digest":     "allow-unsigned: true\nsha256: aaaa",
		"uppercase digest": "allow-unsigned: true\nsha256: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"bad folder":       "allow-unsigned: true\nsha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nintegrations:\n  not a folder: x",
		"bad config":       "allow-unsigned: true\nsha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nconfig: |\n  not: [valid",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Schema.Decode([]byte(raw), "agent.package"); err == nil {
				t.Fatalf("expected a section error for %q", raw)
			}
		})
	}
}

func TestSectionExampleIsValid(t *testing.T) {
	node, err := Schema.Example(nil)
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := Schema.DecodeNode(node, "agent.package")
	if err != nil {
		t.Fatal(err)
	}
	if !digestPattern.MatchString(c.SHA256) {
		t.Fatal("example digest is not a valid pin")
	}
}
