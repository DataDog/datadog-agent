// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package datasecurity contains e2e tests for the packaged Data Security shared-library check.
package datasecurity

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	pgcomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/integration/postgres"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

//go:embed fixtures/datadog-agent.yaml
var agentConfig string

//go:embed fixtures/datasecurity.yaml
var datasecurityConfig string

// postgresScanEnv is a host Agent plus a Dockerized PostgreSQL workload on the same VM.
type postgresScanEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Docker     *components.RemoteHostDocker
}

func postgresScanProvisioner() provisioners.PulumiEnvRunFunc[postgresScanEnv] {
	return func(ctx *pulumi.Context, env *postgresScanEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, "agent-host", ec2.WithInternetAccess())
		if err != nil {
			return err
		}
		if err := host.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
			return err
		}

		manager, err := docker.NewAWSManager(&awsEnv, host)
		if err != nil {
			return err
		}
		if err := manager.Export(ctx, &env.Docker.ManagerOutput); err != nil {
			return err
		}

		pgManifest, pgAssets, err := pgcomp.NewDockerCompose(manager)
		if err != nil {
			return err
		}
		composeDeps := make([]pulumi.ResourceOption, 0, len(pgAssets)+1)
		composeDeps = append(composeDeps, utils.PulumiDependsOn(manager))
		for _, asset := range pgAssets {
			composeDeps = append(composeDeps, utils.PulumiDependsOn(asset))
		}
		pgStack, err := manager.ComposeStrUp("postgres", []docker.ComposeInlineManifest{pgManifest}, nil, composeDeps...)
		if err != nil {
			return err
		}

		agentComp, err := agent.NewHostAgent(&awsEnv, host,
			agentparams.WithAgentConfig(agentConfig),
			agentparams.WithIntegration("datasecurity.d", datasecurityConfig),
			agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(pgStack)),
		)
		if err != nil {
			return err
		}
		return agentComp.Export(ctx, &env.Agent.HostAgentOutput)
	}
}

type postgresScanSuite struct {
	e2e.BaseSuite[postgresScanEnv]
}

func TestDataSecurityPostgresScan(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &postgresScanSuite{}, e2e.WithPulumiProvisioner(postgresScanProvisioner(), nil))
}

func (s *postgresScanSuite) TestPackagedCheckLoadsAndEmitsSDSResult() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"datasecurity", "--json", "--delay", "1000"}))
		require.NoError(c, err, "datasecurity check failed: %s", out)
		assert.NotContains(c, out, "'group' or 'others' have rights on it")
		assert.NotContains(c, out, "no valid check found")

		instance := parseCheckJSON(c, []byte(out))
		assert.Equal(c, 0, instance.Runner.TotalErrors, "datasecurity check reported errors")

		sdsCount := instance.Runner.EventPlatformEvents["sds-result"]
		assert.GreaterOrEqual(c, sdsCount, 1, "expected at least one sds-result event platform event")

		events := instance.SDSResults
		if !assert.NotEmpty(c, events, "aggregator contained no sds-result events") {
			return
		}
		raw := events[0].RawEvent
		assert.Equal(c, "sds-result", events[0].EventType)
		assert.Contains(c, raw, "e2e-datasec-postgres", "sds-result payload missing task_id")
		assert.Contains(c, raw, "accounts", "sds-result payload missing scanned table")
		assert.NotContains(c, raw, "connecting to postgres", "scan failed to connect to postgres")
		// TODO(DATASEC): assert SDS protobuf from fakeintake Client.GetRawPayloads("/api/v2/sdsresult")
	}, 2*time.Minute, 10*time.Second)
}

type epEvent struct {
	EventType string `json:"EventType"`
	RawEvent  string `json:"RawEvent"`
}

type checkInstance struct {
	Aggregator map[string]json.RawMessage `json:"aggregator"`
	Runner     struct {
		TotalErrors         int            `json:"TotalErrors"`
		EventPlatformEvents map[string]int `json:"EventPlatformEvents"`
	} `json:"runner"`
	SDSResults []epEvent
}

func parseCheckJSON(t require.TestingT, check []byte) checkInstance {
	startIdx := bytes.IndexAny(check, "[{")
	require.NotEqual(t, -1, startIdx, "no JSON in check output: %s", string(check))

	var instances []checkInstance
	require.NoError(t, json.Unmarshal(check[startIdx:], &instances), "failed to unmarshal check output: %s", string(check))
	require.NotEmpty(t, instances, "empty check JSON: %s", string(check))

	instance := instances[0]
	if raw, ok := instance.Aggregator["sds-result"]; ok {
		require.NoError(t, json.Unmarshal(raw, &instance.SDSResults))
	}
	return instance
}
