// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2eunit

package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTaskDefinitionDeleter struct {
	requests [][]string
}

func (f *fakeTaskDefinitionDeleter) DeleteTaskDefinitions(_ context.Context, input *ecs.DeleteTaskDefinitionsInput, _ ...func(*ecs.Options)) (*ecs.DeleteTaskDefinitionsOutput, error) {
	f.requests = append(f.requests, input.TaskDefinitions)
	return &ecs.DeleteTaskDefinitionsOutput{}, nil
}

func TestECSTaskDefinitionsFromDeployment(t *testing.T) {
	const providerURN = "urn:pulumi:test::project::pulumi:providers:aws::default"
	const providerID = "provider-id"
	profile, err := json.Marshal("agent-qa")
	require.NoError(t, err)
	deployment, err := json.Marshal(pulumiDeployment{Resources: []pulumiResource{
		{URN: providerURN, Type: "pulumi:providers:aws", ID: providerID, Inputs: map[string]json.RawMessage{"profile": profile}},
		{Type: "aws:s3/bucket:Bucket", ID: "bucket"},
		{Type: ecsTaskDefinitionResourceType, ID: "arn:aws:ecs:us-east-1:123456789012:task-definition/z:1", Provider: providerURN + "::" + providerID},
		{Type: ecsTaskDefinitionResourceType, ID: "arn:aws:ecs:us-east-1:123456789012:task-definition/a:1", Provider: providerURN + "::" + providerID},
		{Type: ecsTaskDefinitionResourceType, ID: "arn:aws:ecs:us-east-1:123456789012:task-definition/a:1", Provider: providerURN + "::" + providerID},
	}})
	require.NoError(t, err)

	arns, err := ecsTaskDefinitionsFromDeployment(deployment)
	require.NoError(t, err)
	assert.Equal(t, []ecsTaskDefinition{
		{ARN: "arn:aws:ecs:us-east-1:123456789012:task-definition/a:1", Profile: "agent-qa"},
		{ARN: "arn:aws:ecs:us-east-1:123456789012:task-definition/z:1", Profile: "agent-qa"},
	}, arns)
}

func TestECSTaskDefinitionsFromDeploymentUsesAmbientCredentialsWithoutProviderProfile(t *testing.T) {
	const providerURN = "urn:pulumi:test::project::pulumi:providers:aws::default"
	const providerID = "provider-id"
	taskDefinitionARN := "arn:aws:ecs:us-east-1:123456789012:task-definition/test:1"
	deployment, err := json.Marshal(pulumiDeployment{Resources: []pulumiResource{
		{URN: providerURN, Type: "pulumi:providers:aws", ID: providerID},
		{Type: ecsTaskDefinitionResourceType, ID: taskDefinitionARN, Provider: providerURN + "::" + providerID},
	}})
	require.NoError(t, err)

	taskDefinitions, err := ecsTaskDefinitionsFromDeployment(deployment)
	require.NoError(t, err)
	require.Equal(t, []ecsTaskDefinition{{ARN: taskDefinitionARN}}, taskDefinitions)
}

func TestDeleteECSTaskDefinitionsBatchesByTen(t *testing.T) {
	fake := &fakeTaskDefinitionDeleter{}
	previousFactory := newECSTaskDefinitionDeleter
	newECSTaskDefinitionDeleter = func(_ context.Context, region, profile string) (ecsTaskDefinitionDeleter, error) {
		assert.Equal(t, "us-east-1", region)
		assert.Equal(t, "agent-qa", profile)
		return fake, nil
	}
	t.Cleanup(func() { newECSTaskDefinitionDeleter = previousFactory })

	taskDefinitions := make([]ecsTaskDefinition, 0, 11)
	for i := range 11 {
		taskDefinitions = append(taskDefinitions, ecsTaskDefinition{
			ARN:     "arn:aws:ecs:us-east-1:123456789012:task-definition/test:" + string(rune('a'+i)),
			Profile: "agent-qa",
		})
	}
	var logs bytes.Buffer
	deleteECSTaskDefinitions(context.Background(), taskDefinitions, &logs)

	require.Len(t, fake.requests, 2)
	assert.Len(t, fake.requests[0], 10)
	assert.Len(t, fake.requests[1], 1)
	assert.Empty(t, logs.String())
}

func TestDeleteECSTaskDefinitionsSkipsMalformedARN(t *testing.T) {
	var logs bytes.Buffer
	deleteECSTaskDefinitions(context.Background(), []ecsTaskDefinition{{ARN: "not-an-arn"}}, &logs)
	assert.Contains(t, logs.String(), "malformed ECS task definition ARN")
}
