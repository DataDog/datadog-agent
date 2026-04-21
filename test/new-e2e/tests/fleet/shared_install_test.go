// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/agent"
)

// testInstallProfile describes how a single test should be prepared.
//
// The goal is to share one agent install across multiple tests in the same
// suite when they need the same install flavor and leave stable state clean.
// On Windows the install/uninstall cycle dominates test time (~2.5 min), so
// skipping it for every shareable test yields several minutes per platform.
//
// Fields:
//
//   - sig: a short identifier for the install flavor. Two tests with the same
//     sig are eligible to share a running agent. Two tests with different sigs
//     will trigger a full reinstall when transitioning.
//   - install: how to install the agent for this test. Called only when a
//     reinstall is needed.
//   - mutator: true when the test leaves stable agent state dirty (e.g. promoted
//     a config experiment, or used install options that cannot be reset without
//     reinstalling). The next test will get a fresh install.
type testInstallProfile struct {
	sig     string
	install func(*agent.Agent)
	mutator bool
}

// defaultInstall installs the agent with no options. Used by most tests.
func defaultInstall(a *agent.Agent) { a.MustInstall() }
