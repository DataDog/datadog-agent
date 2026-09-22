// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file holds the cloud-backed (ec2-host base) attach entry point for
// the ORIGINAL process-manager smoke suite (procmgrLinuxSuite, the same
// suite TestProcmgrSmokeLinuxSuite runs on a Pulumi-provisioned EC2 VM). The
// environment contract is e2ectl-host-procmgr.yml next to this file. The
// suite stays host-class on purpose: its assertions are about the
// dd-procmgrd systemd unit and the processes it supervises.
package procmgr

import (
	"testing"
)

// TestProcmgrSmokeLinuxSuiteOnHost is the migration contract for the procmgr
// smoke suite on an e2ectl ec2-host environment: it will run the ORIGINAL
// procmgrLinuxSuite body via
//
//	e2ectlenv.Attach[environments.Host](e2ectlenv.RequireEnv(t))
//
// It SKIPS today because the suite's install-time provisioning cannot be
// expressed by the script installer: it places three processes.d config
// files under /opt/datadog-agent/processes.d (test-sleep.yaml,
// datadog-agent-ddot.yaml, missing-binary.yaml — agentparams.WithFile),
// which the agent.script section (datadog.yaml extras + conf.d integrations
// only) has no equivalent for. The DDOT part of SetupSuite installs the
// datadog-agent-ddot package over apt at runtime, which needs the same
// systemd host this base provides. Remove this skip when the script
// installer grows an arbitrary-file input (or a processes.d section).
func TestProcmgrSmokeLinuxSuiteOnHost(t *testing.T) {
	t.Skip("pending: processes.d config file provisioning in the agent.script installer; see the boundary comment on this entry point")
}
