// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/zorkian/go-datadog-api"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
)

// ECS is an environment that contains a ECS deployed in a cluster, FakeIntake and Agent configured to talk to each other.
type ECS struct {
	AwsEnvironment *aws.Environment
	ClusterName    pulumi.StringInput
	ClusterArn     pulumi.StringInput

	// Components
	FakeIntake    *components.FakeIntake
	DatadogClient *datadog.Client
}

var _ e2e.Initializable = &ECS{}

// Init initializes the environment
func (e *ECS) Init(_ e2e.Context) error {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if err != nil {
		return err
	}
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	if err != nil {
		return err
	}
	e.DatadogClient = datadog.NewClient(apiKey, appKey)
	return nil
}
