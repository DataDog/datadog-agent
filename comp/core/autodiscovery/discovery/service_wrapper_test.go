// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import "testing"

func TestServiceWithProbeResult_GetExtraConfig(t *testing.T) {
	base := &fakeService{id: "svc"}
	w := WrapWithProbeResult(base, ProbeResult{Port: 8090})

	v, err := w.GetExtraConfig("discovered_port")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if v != "8090" {
		t.Fatalf("got %q want 8090", v)
	}

	if _, err := w.GetExtraConfig("unknown"); err == nil {
		t.Fatal("expected error for unknown extra key")
	}
}
