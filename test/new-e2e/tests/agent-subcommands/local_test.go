// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

// The local attach entry for the health suite lives in health_nix_test.go
// (TestLinuxHealthSuiteOnLocal attaches linuxHealthSuite, the same suite
// TestLinuxHealthSuite runs on an EC2 VM) against a local e2ectl container
// environment started from e2ectl-local.yml next to this file:
//
//	e2ectl start --config e2ectl-local.yml --name <name>
//	e2ectl install -env <name>
//	e2ectl test -env <name> --suite ./test/new-e2e/tests/agent-subcommands/ \
//	  --run TestLinuxHealthSuiteOnLocal
//
// The install builds the agent binary from the working tree (dda inv
// agent.build), pins it into a container on the environment's network and
// records its path — the AgentClient invokes it directly, no sudo.
// TestDefaultInstallUnhealthy skips itself in attach mode (see
// health_common_test.go): it re-provisions the host with UpdateEnv, which an
// attached environment does not do.
//
// The status suite is NOT attached on purpose: its assertions describe a
// full OS install — the APM agent section expects "Status: Running" (the
// trace-agent is a separate process the single core binary does not provide)
// and Autodiscovery is expected absent (the EC2 host has no container
// features; the local agent container enables them). Attaching it would be a
// failing-by-design entry point. When the binary install grows a trace-agent
// and a host-shaped (non-container) AD mode, the same plug applies.
