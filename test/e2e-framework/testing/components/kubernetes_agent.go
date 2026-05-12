// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
)

// KubernetesAgentInstaller is the interface that agent installers (Helm,
// Operator, etc.) implement to support reconfiguration via Configure.
type KubernetesAgentInstaller interface {
	// Upgrade reconfigures the agent with the given options.
	// For Helm this runs helm upgrade; for Operator this updates the CR.
	Upgrade(t *testing.T, opts []kubernetesagentparams.Option) error
}

// KubernetesAgent is an agent running in a Kubernetes cluster.
type KubernetesAgent struct {
	agent.KubernetesAgentOutput

	// Installer is set by the install function (helmagent.Install,
	// operatoragent.Install, etc.) and determines how Configure
	// reconfigures the agent.
	Installer KubernetesAgentInstaller

	// baseOptions stores the agent options from the initial installation.
	// Configure merges new options on top of these.
	baseOptions []kubernetesagentparams.Option
}

// SetBaseOptions stores the baseline agent options from the initial install.
func (a *KubernetesAgent) SetBaseOptions(opts ...kubernetesagentparams.Option) {
	a.baseOptions = opts
}

// Configure reconfigures the agent with new options, merging them on top
// of the baseline options from the initial installation.
//
// The actual reconfiguration is performed by the Installer that was set
// during initial installation (e.g., Helm runs helm upgrade, Operator
// updates the DatadogAgent CR).
func (a *KubernetesAgent) Configure(t *testing.T, opts ...kubernetesagentparams.Option) {
	t.Helper()
	if a.Installer == nil {
		t.Fatal("KubernetesAgent.Configure: no installer set, was the agent installed via helmagent.Install or similar?")
	}

	// Merge: apply baseline options first, then caller's overrides
	merged := make([]kubernetesagentparams.Option, 0, len(a.baseOptions)+len(opts))
	merged = append(merged, a.baseOptions...)
	merged = append(merged, opts...)

	if err := a.Installer.Upgrade(t, merged); err != nil {
		t.Fatalf("KubernetesAgent.Configure failed: %v", err)
	}
}
