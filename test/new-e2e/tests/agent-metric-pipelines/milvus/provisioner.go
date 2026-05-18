// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package milvus provides an E2E scenario that deploys Milvus, drives real
// traffic against it, runs a Docker Agent configured with the Milvus
// integration via Autodiscovery, and expects metrics to flow to a real
// Datadog intake (no fakeintake).
package milvus

import (
	_ "embed"
	"encoding/base64"
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
)

//go:embed testfixtures/docker-compose.milvus.yaml
var milvusComposeContent string

//go:embed testfixtures/traffic.py
var trafficScript string

// Provisioner returns the stock AWS Docker host provisioner configured for
// the Milvus E2E scenario:
//
//   - WithoutFakeIntake() — the Agent ships to the real Datadog intake
//     using the runner's API key.
//   - WithExtraComposeManifest() — adds Milvus standalone (etcd + MinIO +
//     milvus) and a pymilvus traffic generator to the agent compose.
//   - WithEnvironmentVariables() — injects per-run state into compose:
//     a unique e2e_test_id (also surfaced via DD_TAGS / AD labels) and the
//     base64-encoded traffic generator script.
//   - WithTags() — host-level tags attached to all metrics from this run.
//
// The Milvus integration itself is configured via Datadog Autodiscovery
// labels on the milvus service (see the embedded compose), so no host-side
// conf.d file is needed.
func Provisioner(testID string) provisioners.TypedProvisioner[environments.DockerHost] {
	trafficB64 := base64.StdEncoding.EncodeToString([]byte(trafficScript))

	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithExtraComposeManifest(
			"milvus",
			pulumi.String(milvusComposeContent),
		),
		dockeragentparams.WithEnvironmentVariables(pulumi.StringMap{
			"DD_E2E_TEST_ID":        pulumi.String(testID),
			"DD_MILVUS_TRAFFIC_B64": pulumi.String(trafficB64),
		}),
		dockeragentparams.WithTags([]string{
			"env:e2e",
			"e2e_scenario:milvus",
			"e2e_test_id:" + testID,
		}),
	}

	// If the caller (typically scripts/_lib.sh after dd-auth) selected a
	// non-default Datadog site, pin it on the agent container so the agent
	// ships to the same org that owns the API key.
	if site := os.Getenv("MILVUS_DD_SITE"); site != "" {
		agentOpts = append(agentOpts,
			dockeragentparams.WithAgentServiceEnvVariable("DD_SITE", pulumi.String(site)),
		)
	}

	return awsdocker.Provisioner(
		awsdocker.WithRunOptions(
			ec2docker.WithoutFakeIntake(),
			ec2docker.WithAgentOptions(agentOpts...),
		),
	)
}
