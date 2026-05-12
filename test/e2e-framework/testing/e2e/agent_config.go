// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

// SetAgentConfig reconfigures the agent on a provisioned Host environment
// without re-running Pulumi. It delegates to the agent component's Configure
// method, which merges the given options with the baseline options from the
// initial configuration.
//
// Usage:
//
//	e2e.SetAgentConfig(s.T(), s.Env(),
//	    agentparams.WithAgentConfig("log_level: info"),
//	)
//
// Deprecated: Use s.Env().Agent.Configure(s.T(), opts...) directly instead.
// This function is kept for backward compatibility during migration.
func SetAgentConfig(t *testing.T, env *environments.Host, opts ...agentparams.Option) {
	t.Helper()
	env.Agent.Configure(t, opts...)
}
