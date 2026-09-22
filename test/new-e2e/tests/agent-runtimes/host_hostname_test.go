// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file holds the cloud-backed (ec2-host base) attach entry point for the
// ORIGINAL hostname IMDSv2 transition suite (baseHostnameSuite, the same
// suite TestBaseHostnameSuite runs on a Pulumi-provisioned EC2 VM). The
// environment contract is e2ectl-host-hostname.yml next to this file.
package agentruntimes

import (
	"testing"
)

// TestBaseHostnameSuiteOnHost is the migration contract for the hostname
// IMDSv2 suite on an e2ectl ec2-host environment: it will run the ORIGINAL
// baseHostnameSuite body via
//
//	e2ectlenv.Attach[environments.Host](e2ectlenv.RequireEnv(t))
//
// It SKIPS today because every subtest re-provisions the environment before
// asserting: runHostnameTest calls s.UpdateEnv(awshost.ProvisionerNoFakeIntake(...))
// with a per-case agent config AND, for TestWithoutIMDSv1, the EC2 instance
// option ec2.WithIMDSv1Disable(). Neither is expressible under attach — the
// config change needs the mutator seam (gap-analysis Step 1,
// e2ectlenv.ApplyAgentConfig), and IMDSv1-disable is an EC2 instance
// property, not agent config, so it needs an e2ectl capability field that
// does not exist yet. Running the attach below would fail on the first
// subtest. Remove this skip when both seams exist.
func TestBaseHostnameSuiteOnHost(t *testing.T) {
	t.Skip("pending: per-subtest agent-config mutation (gap-analysis Step 1) and an EC2 IMDSv1-disable capability in e2ectl; see the boundary comment on this entry point")
}
