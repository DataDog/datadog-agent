// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package provisioners

import (
	"encoding/json"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func TestRawResourcesFromOutputs(t *testing.T) {
	outputs := auto.OutputMap{
		"host": {
			Value: map[string]any{
				"address": "1.2.3.4",
				"port":    22,
			},
			Secret: false,
		},
		"version": {
			// Non-map (string) output — should be skipped.
			Value:  "7.55.0",
			Secret: false,
		},
	}

	resources, err := rawResourcesFromOutputs(outputs)
	if err != nil {
		t.Fatalf("rawResourcesFromOutputs: %v", err)
	}

	// "host" must be present and contain valid JSON.
	raw, ok := resources["host"]
	if !ok {
		t.Fatal("expected 'host' key in resources")
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("'host' value is not valid JSON: %v", err)
	}
	if decoded["address"] != "1.2.3.4" {
		t.Fatalf("unexpected address: %v", decoded["address"])
	}

	// "version" must be absent (non-map output is skipped).
	if _, ok := resources["version"]; ok {
		t.Fatal("expected 'version' key to be absent (non-map output should be skipped)")
	}
}
