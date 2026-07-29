// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import "testing"

func TestLifecycleUnknownScenario(t *testing.T) {
	resetRegistry()
	if err := Create(nil, "ghost", "s", nil); err == nil {
		t.Fatal("Create: expected unknown-scenario error")
	}
	if err := RunAction(nil, "ghost", "s", "a", nil); err == nil {
		t.Fatal("RunAction: expected unknown-scenario error")
	}
	if err := Destroy(nil, "ghost", "s"); err == nil {
		t.Fatal("Destroy: expected unknown-scenario error")
	}
}
