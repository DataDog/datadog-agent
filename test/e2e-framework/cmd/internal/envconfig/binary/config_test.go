// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package binary

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
)

func TestSectionRoundTrip(t *testing.T) {
	c, _, err := Schema.Decode([]byte("runtime-image: registry.datadoghq.com/agent:7.83.0\nconfig: |\n  log_level: debug\n"), "agent.binary")
	if err != nil {
		t.Fatal(err)
	}
	if c.RuntimeImage != "registry.datadoghq.com/agent:7.83.0" {
		t.Fatalf("unexpected section: %+v", c)
	}
}

func TestSectionHasNoVersionOrImage(t *testing.T) {
	// The binary installer always builds from source: version/image belong to
	// the script/helm sections and must be unknown here — the type system
	// enforcing what runtime rejection used to.
	for name, raw := range map[string]string{
		"version": "version: \"7.69.0\"",
		"image":   "image: gcr.io/datadoghq/agent:7.99.0-e2ectl",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Schema.Decode([]byte(raw), "agent.binary"); err == nil {
				t.Fatalf("expected an unknown-field error for %q", raw)
			}
		})
	}
}

func TestSectionRules(t *testing.T) {
	for name, raw := range map[string]string{
		"bad folder": "integrations:\n  not a folder: x",
		"bad config": "config: |\n  not: [valid",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Schema.Decode([]byte(raw), "agent.binary"); err == nil {
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
	if _, _, err := Schema.DecodeNode(node, "agent.binary"); err != nil {
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
