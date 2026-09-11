// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package script

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
)

func TestSectionRoundTrip(t *testing.T) {
	c, _, err := Schema.Decode([]byte(`version: "7.69.0"
integrations:
  custom_logs.d: |
    logs:
      - type: file
`), "agent.script")
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != "7.69.0" || len(c.Integrations) != 1 {
		t.Fatalf("unexpected section: %+v", c)
	}
}

func TestSectionRules(t *testing.T) {
	for name, raw := range map[string]string{
		"missing version": "",
		"bad version":     "version: 7.x",
		"unknown field":   "version: \"7.69.0\"\nimage: somewhere:tag",
		"bad folder":      "version: \"7.69.0\"\nintegrations:\n  not a folder: x",
		"bad config":      "version: \"7.69.0\"\nconfig: |\n  not: [valid",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Schema.Decode([]byte(raw), "agent.script"); err == nil {
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
	data, err := configschema.Encode(node)
	if err != nil {
		t.Fatal(err)
	}
	// The example shows a real, editable version — not an invalid placeholder.
	if !strings.Contains(string(data), "7.69.0") {
		t.Fatalf("example should show the version example, got: %s", data)
	}
	if _, _, err := Schema.DecodeNode(node, "agent.script"); err != nil {
		t.Fatal(err)
	}
}
