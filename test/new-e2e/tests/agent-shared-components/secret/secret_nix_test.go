// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// package secret contains e2e tests for secret management
package secret

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type linuxRuntimeSecretSuite struct {
	baseRuntimeSecretSuite
}

func TestLinuxRuntimeSecretSuite(t *testing.T) {
	e2e.Run(t, &linuxRuntimeSecretSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

//go:embed fixtures/secret_script.py
var secretScript []byte

func (v *linuxRuntimeSecretSuite) TestSecretRuntimeAPIKey() {
	config := `secret_backend_command: /tmp/bin/secret.sh
hostname: ENC[hostname]`

	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithFile("/tmp/bin/secret.sh", string(secretScript), true),
		),
	))

	v.Env().RemoteHost.MustExecute(`sudo sh -c "chown dd-agent:dd-agent /tmp/bin/secret.sh && chmod 700 /tmp/bin/secret.sh"`)
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(config),
			agentparams.WithFile("/tmp/bin/secret.sh", string(secretScript), true),
		),
	))

	assert.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		checks, err := v.Env().FakeIntake.Client().GetCheckRun("datadog.agent.up")
		require.NoError(t, err)
		if assert.NotEmpty(t, checks) {
			assert.Equal(t, "e2e.test", checks[len(checks)-1].HostName)
		}
	}, 30*time.Second, 2*time.Second)
}
