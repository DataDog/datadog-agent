// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package params

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
)

func TestFakeintakeParamsToOptions(t *testing.T) {
	// sentinel option — a no-op func that matches fakeintake.Option (= func(*fakeintake.Params) error)
	sentinel := fakeintake.Option(func(p *fakeintake.Params) error { return nil })

	// Case 1: Enabled=true with one AdvancedOption — should pass through exactly one option.
	f := FakeintakeParams{
		Enabled:         true,
		AdvancedOptions: []fakeintake.Option{sentinel},
	}
	opts, err := f.ToOptions()
	if err != nil {
		t.Fatalf("ToOptions (case 1): %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("want 1 option, got %d", len(opts))
	}

	// Case 2: empty FakeintakeParams — ToOptions returns no error and zero options.
	empty := FakeintakeParams{}
	opts2, err2 := empty.ToOptions()
	if err2 != nil {
		t.Fatalf("ToOptions (case 2): %v", err2)
	}
	if len(opts2) != 0 {
		t.Fatalf("want 0 options, got %d", len(opts2))
	}
}
