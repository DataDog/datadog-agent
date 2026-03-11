// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package aws

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/internal/awsutil"
	"github.com/stretchr/testify/assert"
)

// ssmMockClient is the struct we'll use to mock the SSM client
// for unit tests. E2E tests should be written with the real client.
type ssmMockClient struct {
	parameters map[string]string
}

func (c *ssmMockClient) GetParameter(_ context.Context, name string, _ bool) (*string, error) {
	if value, exists := c.parameters[name]; exists {
		return &value, nil
	}
	return nil, fmt.Errorf("parameter %s not found", name)
}

func TestSSMParameterStoreBackend_ParametersByPath(t *testing.T) {
	mockClient := &ssmMockClient{
		parameters: map[string]string{
			"/group1/key1":      "value1",
			"/group1/nest/key2": "value2",
			"/group2/key3":      "value3",
		},
	}
	getSSMClient = func(_ *awsutil.AWSConfig) ssmClient {
		return mockClient
	}

	ssmParameterStoreSecretsBackend, err := NewSSMParameterStoreBackend(map[string]interface{}{
		"backend_type": "aws.ssm",
		"aws_session":  dummyAWSSession(),
	})
	assert.NoError(t, err)

	ctx := context.Background()
	secretOutput := ssmParameterStoreSecretsBackend.GetSecretOutput(ctx, "/group1/key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = ssmParameterStoreSecretsBackend.GetSecretOutput(ctx, "/group1/nest/key2")
	assert.Equal(t, "value2", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = ssmParameterStoreSecretsBackend.GetSecretOutput(ctx, "/group1/key_noexist")
	assert.Nil(t, secretOutput.Value)
	assert.NotNil(t, secretOutput.Error)
	assert.Contains(t, *secretOutput.Error, "not found")

	secretOutput = ssmParameterStoreSecretsBackend.GetSecretOutput(ctx, "/group2/key3")
	assert.Equal(t, "value3", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)
}
