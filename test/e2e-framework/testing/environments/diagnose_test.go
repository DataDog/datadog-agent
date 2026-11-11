// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package environments

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// stubAgent stands in for an agent client that Diagnose must not reach.
type stubAgent struct{ agentclient.Agent }

// TestDiagnoseNilClient diagnoses environments whose provisioning failed: the
// components exist, but a client the flare needs is nil.
func TestDiagnoseNilClient(t *testing.T) {
	envs := map[string]common.Diagnosable{
		"DockerHost agent client":  &DockerHost{RemoteHost: &components.RemoteHost{}, Docker: &components.RemoteHostDocker{}, Agent: &components.DockerAgent{}},
		"DockerHost docker client": &DockerHost{RemoteHost: &components.RemoteHost{}, Docker: &components.RemoteHostDocker{}, Agent: &components.DockerAgent{Client: stubAgent{}}},
		"Host agent client":        &Host{RemoteHost: &components.RemoteHost{}, Agent: &components.RemoteHostAgent{}},
		"Host remote host client":  &Host{RemoteHost: &components.RemoteHost{}, Agent: &components.RemoteHostAgent{Client: stubAgent{}}},
	}
	for name, env := range envs {
		t.Run(name, func(t *testing.T) {
			_, err := env.Diagnose(t.TempDir())
			assert.ErrorContains(t, err, "not initialized")
		})
	}
}
