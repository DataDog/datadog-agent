// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file adds the local attach entry point for the ORIGINAL cluster-agent
// secret-generic-connector suite (clusterAgentBinarySuite, the same suite
// TestClusterAgentBinarySuite runs on a Pulumi-provisioned kind-on-EC2
// environment). Unlike the other containers suites in the cloud group, this
// one runs on kind in its Pulumi form too (the kindvm provisioner), and the
// e2ectl kind base already covers what it needs: the Helm installer targets
// the "datadog" namespace the suite lists pods in
// (cmd/e2ectl/internal/installer/installer.go params.Namespace), and
// environment.fakeintake: false mirrors scenkind.WithoutFakeIntake. The
// environment comes from e2ectl-kind-sgc.yml next to this file.
//
// Config + entry only: not validated in this migration pass.
package containers

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	e2ectlenv "github.com/DataDog/datadog-agent/test/new-e2e/utils/e2ectlenv"
)

// TestClusterAgentSGCBinaryOnLocalKind runs the ORIGINAL
// secret-generic-connector suite against a local e2ectl kind environment
// started from e2ectl-kind-sgc.yml. The suite's single test asserts the
// binary's presence, permissions and ownership inside the cluster-agent pod —
// no fakeintake, no workloads.
//
// Known attach-mode boundary: the suite reads the cluster-agent pod selector
// from suite.Env().Agent.LinuxClusterAgent, which the e2ectl Helm installer
// must record in the snapshot the same way the Pulumi path does. That wiring
// is part of what the first live run of this entry will verify.
func TestClusterAgentSGCBinaryOnLocalKind(t *testing.T) {
	envName := e2ectlenv.RequireEnv(t)
	e2ectlenv.RequireSnapshot(t, envName)
	t.Parallel()
	e2e.Run(t, &clusterAgentBinarySuite{}, e2e.WithProvisioner(
		e2ectlenv.Attach[environments.Kubernetes](envName),
	))
}
