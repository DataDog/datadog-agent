// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file holds the local attach entry point for the ORIGINAL rtloader
// multiprocessing suite (linuxMultiProcessingLibSuite, the same suite
// TestLinuxMultiProcessingLibSuite runs on an EC2 VM). It documents the
// attach-mode boundary instead of running a suite that cannot be provisioned
// honestly today.
package agentruntimes

import (
	"testing"
)

// TestLinuxMultiProcessingLibSuiteOnLocal is the migration contract for the
// rtloader multiprocessing suite on a local e2ectl binary-agent environment
// (base local, agent.install binary): it will run the ORIGINAL
// linuxMultiProcessingLibSuite body via
//
//	e2ectlenv.Attach[environments.Host](e2ectlenv.RequireEnv(t))
//
// It SKIPS today because the suite's install-time provisioning cannot be
// expressed by the binary installer: the conf.d integration
// (multi_pid_check.d) maps to agent.binary.integrations, but the custom
// Python check the integration runs — /etc/datadog-agent/checks.d/
// multi_pid_check.py, provisioned by agentparams.WithFile — has no
// agent.binary equivalent (the section covers datadog.yaml extras and conf.d
// integrations only, not arbitrary files). Without the Python file the check
// cannot run, so attaching would be a failing-by-design entry point. Remove
// this skip when the local binary installer grows a checks.d/custom-file
// input (see qa-plans/pending/qa-e2ectl-test-migration-gap-analysis.md).
func TestLinuxMultiProcessingLibSuiteOnLocal(t *testing.T) {
	t.Skip("pending: the local binary installer cannot deploy custom checks.d Python files; see the boundary comment on this entry point")
}
