// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file adds an entry point that runs the ORIGINAL containers kind suite
// (kindSuite, the same suite TestKindSuite runs on a Pulumi-provisioned
// kind-on-EC2 environment) against a local e2ectl kind environment — the same
// test bodies, only the provisioning differs. The environment comes from
// e2ectl-kind.yml next to this file (see its header for the exact commands).
//
// This is the migration pattern: the suite and its assertions are unchanged;
// the environment is described by a config file next to the test instead of
// Pulumi scenario options, and the entry point attaches to a live
// environment instead of provisioning one.
package containers

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	e2ectlenv "github.com/DataDog/datadog-agent/test/new-e2e/utils/e2ectlenv"
)

// TestKindSuiteOnLocalKind runs the ORIGINAL kind suite against a local
// e2ectl kind environment.
//
// Known attach-mode gaps (see the qa-plans index): TestArgoRollout needs the
// Argo Rollouts controller (not yet an e2ectl workload); TestHostTags
// hard-asserts arch:amd64; TestVersion compares image tags against the
// scenario's expectations.
func TestKindSuiteOnLocalKind(t *testing.T) {
	envName := e2ectlenv.RequireEnv(t)
	e2ectlenv.RequireSnapshot(t, envName)
	t.Parallel()
	e2e.Run(t, &kindSuite{}, e2e.WithProvisioner(
		e2ectlenv.Attach[environments.Kubernetes](envName),
	))
}
