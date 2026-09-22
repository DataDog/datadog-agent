// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file adds an entry point that runs the ORIGINAL agent-runtimes
// infra_basic suite (basicLinuxSuite, the same suite TestBasicLinuxSuite runs
// on an EC2 VM) against a local binary agent managed by e2ectl — the same
// test bodies, only the provisioning differs. The environment comes from
// e2ectl-local.yml next to this file (see its header for the exact commands).
//
// This is the migration pattern: the suite and its assertions are unchanged;
// the agentparams agent config and per-check integration configs become the
// binary installer's agent.binary.config and agent.binary.integrations, and
// the entry point attaches to a live environment instead of provisioning one.
package agentruntimes

import (
	"testing"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	e2ectlenv "github.com/DataDog/datadog-agent/test/new-e2e/utils/e2ectlenv"
)

// localBasicLinuxSuite runs the existing infra_basic suite against the local
// binary agent. The descriptor stays Linux — the binary agent runs in the
// pinned official Agent image (Ubuntu-based), so the suite's OS filtering
// (the Linux-only "load" check) matches the original Linux entry point.
type localBasicLinuxSuite struct {
	basicSuite
}

// TestBasicLinuxSuiteOnLocal runs the ORIGINAL infra_basic suite against a
// local e2ectl binary-agent environment.
//
// Known attach-mode boundary: TestAdditionalCheckWorks re-provisions the
// agent mid-suite (s.UpdateEnv with the awshost Pulumi provisioner) to add
// the http_check/custom_mycheck configs — under attach that is a no-op at
// best and a Pulumi invocation at worst, never a live config change. It is
// pending the config mutator seam (gap-analysis Step 1,
// e2ectlenv.ApplyAgentConfig); select the attachable part with
// `-run '^TestBasicLinuxSuiteOnLocal$/^TestCheckSchedulingBehavior$'`.
func TestBasicLinuxSuiteOnLocal(t *testing.T) {
	envName := e2ectlenv.RequireEnv(t)
	e2ectlenv.RequireSnapshot(t, envName)
	t.Parallel()
	e2e.Run(t, &localBasicLinuxSuite{
		basicSuite{
			descriptor: e2eos.Ubuntu2204,
		},
	}, e2e.WithProvisioner(
		e2ectlenv.Attach[environments.Host](envName),
	))
}
