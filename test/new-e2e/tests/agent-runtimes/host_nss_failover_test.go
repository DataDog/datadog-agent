// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file holds the attach entry point for the ORIGINAL forwarder NSS
// failover suite (multiFakeIntakeSuite, the same suite TestMultiFakeintakeSuite
// runs behind an inline Pulumi program). No config is possible yet: the suite
// needs the custom multiFakeIntakeEnv topology — one EC2 host with the agent
// and a docker manager, plus TWO fakeintakes reached through a switchable
// logical hostname — which the pending custom-environments plan is the seam
// for (qa-plans/pending/qa-e2ectl-custom-environments-plan.md). One fakeintake
// per environment is the e2ectl default today; a second intake, the /etc/hosts
// logical-name switch, and the docker workload driver on the agent host are
// all beyond the current config surface.
package agentruntimes

import (
	"testing"
)

// TestMultiFakeintakeSuiteOnHost is the migration contract for the NSS
// failover suite: it will run the ORIGINAL multiFakeIntakeSuite body against
// a custom e2ectl environment expressing multiFakeIntakeEnv, once the
// custom-environments plan lands the multi-fakeintake topology. Until then
// there is no environment type to attach (e2ectlenv.Attach binds the stock
// environments only), so the entry skips rather than attaching a single-
// intake environment that would fail every failover assertion.
func TestMultiFakeintakeSuiteOnHost(t *testing.T) {
	t.Skip("pending: the multi-fakeintake custom environment (qa-plans/pending/qa-e2ectl-custom-environments-plan.md); see the boundary comment on this entry point")
}
