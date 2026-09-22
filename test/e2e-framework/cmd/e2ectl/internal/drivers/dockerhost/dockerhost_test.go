// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dockerhost

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
)

// The lifecycle itself (executor handoff, fakeintake bookkeeping) is proven in
// the shared pulumiworker package; this file keeps the docker-host wiring honest.
func TestDriverRegistrationWiring(t *testing.T) {
	d := New()
	if d.ID() != workerclient.BaseDockerHost {
		t.Fatalf("driver ID %q must be the shared base spelling %q", d.ID(), workerclient.BaseDockerHost)
	}
	if d.Description() == "" {
		t.Fatal("the registry requires a description")
	}
	ids := make([]string, 0)
	for _, i := range d.Installers() {
		ids = append(ids, i.ID())
	}
	if len(ids) != 2 || ids[0] != "script" || ids[1] != "package" {
		t.Fatalf("unexpected installers: %v", ids)
	}
}
