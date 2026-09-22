// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file holds the local attach entry point for the ORIGINAL shared
// library check suite (linuxSharedLibrarySuite, the same suite
// TestLinuxSharedLibraryCheckSuite runs on an EC2 VM). It documents the
// attach-mode boundary instead of running a suite that cannot be provisioned
// honestly today.
package agentruntimes

import (
	"testing"
)

// TestLinuxSharedLibraryCheckSuiteOnLocal is the migration contract for the
// shared-library check suite on a local e2ectl binary-agent environment: it
// will run the ORIGINAL linuxSharedLibraryCheckSuite body via
//
//	e2ectlenv.Attach[environments.Host](e2ectlenv.RequireEnv(t))
//
// It SKIPS today because both of the suite's tests mutate the environment
// mid-suite before asserting: updateEnvWithCheckConfigAndSharedLibrary calls
// s.UpdateEnv with the awshost Pulumi provisioner to (a) copy a different
// libdatadog-agent-*.so into the checks.d folder per test and (b) apply the
// matching check config. Under attach, UpdateEnv with a Pulumi provisioner is
// at best a no-op against the live agent and at worst a cloud invocation —
// never the intended change. Two seams are pending: the config mutator
// (gap-analysis Step 1, e2ectlenv.ApplyAgentConfig) and binary (.so) file
// placement in the local installer. Remove this skip when both exist.
func TestLinuxSharedLibraryCheckSuiteOnLocal(t *testing.T) {
	t.Skip("pending: per-subtest UpdateEnv config mutation (gap-analysis Step 1) plus shared-library (.so) file deployment; see the boundary comment on this entry point")
}
