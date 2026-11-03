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
	deployment, err := json.Marshal(pulumiDeployment{Resources: []pulumiResource{
		{Type: "aws:s3/bucket:Bucket", ID: "bucket"},
		{Type: ecsTaskDefinitionResourceType, ID: "arn:aws:ecs:us-east-1:123456789012:task-definition/z:1"},
		{Type: ecsTaskDefinitionResourceType, ID: "arn:aws:ecs:us-east-1:123456789012:task-definition/a:1"},
		{Type: ecsTaskDefinitionResourceType, ID: "arn:aws:ecs:us-east-1:123456789012:task-definition/a:1"},
	}})
	require.NoError(t, err)

	arns, err := ecsTaskDefinitionsFromDeployment(deployment)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"arn:aws:ecs:us-east-1:123456789012:task-definition/a:1",
		"arn:aws:ecs:us-east-1:123456789012:task-definition/z:1",
	}, arns)
}

func TestDeleteECSTaskDefinitionsBatchesByTen(t *testing.T) {
	fake := &fakeTaskDefinitionDeleter{}
	previousFactory := newECSTaskDefinitionDeleter
	newECSTaskDefinitionDeleter = func(_ context.Context, region string) (ecsTaskDefinitionDeleter, error) {
		assert.Equal(t, "us-east-1", region)
		return fake, nil
	}
	t.Cleanup(func() { newECSTaskDefinitionDeleter = previousFactory })

	arns := make([]string, 0, 11)
	for i := range 11 {
		arns = append(arns, "arn:aws:ecs:us-east-1:123456789012:task-definition/test:"+string(rune('a'+i)))
	}
	var logs bytes.Buffer
	deleteECSTaskDefinitions(context.Background(), arns, &logs)

	require.Len(t, fake.requests, 2)
	assert.Len(t, fake.requests[0], 10)
	assert.Len(t, fake.requests[1], 1)
	assert.Empty(t, logs.String())
}

func TestDeleteECSTaskDefinitionsSkipsMalformedARN(t *testing.T) {
	var logs bytes.Buffer
	deleteECSTaskDefinitions(context.Background(), []string{"not-an-arn"}, &logs)
	assert.Contains(t, logs.String(), "malformed ECS task definition ARN")
}
