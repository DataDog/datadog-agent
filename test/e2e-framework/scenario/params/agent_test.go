// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package params

import "testing"

func TestAgentParamsToOptions(t *testing.T) {
	a := AgentParams{Version: "7.42.0", Flavor: "datadog-fips-agent", Install: true}
	opts, err := a.ToOptions()
	if err != nil {
		t.Fatalf("ToOptions: %v", err)
	}
	// version + flavor => 2 options
	if len(opts) != 2 {
		t.Fatalf("want 2 options, got %d", len(opts))
	}
}

func TestAgentParamsConfigPathMissing(t *testing.T) {
	a := AgentParams{ConfigPath: "/does/not/exist.yaml", Install: true}
	if _, err := a.ToOptions(); err == nil {
		t.Fatal("expected error reading missing config path")
	}
}
