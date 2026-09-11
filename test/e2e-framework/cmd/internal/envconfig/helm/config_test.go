// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
)

func TestSectionRoundTrip(t *testing.T) {
	c, _, err := Schema.Decode([]byte("image: gcr.io/datadoghq/agent:7.99.0-e2ectl"), "agent.helm")
	if err != nil {
		t.Fatal(err)
	}
	if c.Image != "gcr.io/datadoghq/agent:7.99.0-e2ectl" || c.Version != "" {
		t.Fatalf("unexpected section: %+v", c)
	}
}

// TestSectionImageRules pins the checks that used to be global config rules:
// they now live only where the image field exists.
func TestSectionImageRules(t *testing.T) {
	for name, raw := range map[string]string{
		"version or image required": "",
		"bad version":               "version: 7.x",
		"missing registry":          "image: agent:7.99.0-e2ectl",
		"non-semver tag":            "image: gcr.io/datadoghq/agent:e2ectl-dev",
		"unknown field":             "version: \"7.69.0\"\nconfig: x",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := Schema.Decode([]byte(raw), "agent.helm")
			if err == nil {
				t.Fatalf("expected a section error for %q", raw)
			}
		})
	}
	if _, _, err := Schema.Decode([]byte("version: \"7.69.0\"\nimage: gcr.io/datadoghq/agent:7.99.0-e2ectl"), "agent.helm"); err != nil {
		t.Fatalf("version and image may coexist (image wins at install): %v", err)
	}
}

func TestSectionErrorAnchors(t *testing.T) {
	_, _, err := Schema.Decode([]byte("image: gcr.io/datadoghq/agent:e2ectl-dev"), "agent.helm")
	if err == nil || !strings.Contains(err.Error(), "semver-shaped") {
		t.Fatalf("expected the semver-shaped tag rule, got: %v", err)
	}
	_, _, err = Schema.Decode([]byte(""), "agent.helm")
	if err == nil || !strings.Contains(err.Error(), "either version or image") {
		t.Fatalf("expected the version-or-image rule, got: %v", err)
	}
}

func TestSectionExampleIsValid(t *testing.T) {
	node, err := Schema.Example(nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := configschema.Encode(node)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "7.69.0") {
		t.Fatalf("example should show the version example, got: %s", data)
	}
	if _, _, err := Schema.DecodeNode(node, "agent.helm"); err != nil {
		t.Fatal(err)
	}
}
