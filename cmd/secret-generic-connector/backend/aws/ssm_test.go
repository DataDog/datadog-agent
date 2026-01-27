// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package aws

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
)

// ssmMockClient is the struct we'll use to mock the SSM client
// for unit tests. E2E tests should be written with the real client.
type ssmMockClient struct {
	parameters map[string]interface{}
}

func (c *ssmMockClient) GetParametersByPath(_ context.Context, params *ssm.GetParametersByPathInput, _ ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error) {
	if params == nil || params.Path == nil {
		return nil, nil
	}
	outParameters := []types.Parameter{}
	for key, value := range c.parameters {
		if strings.HasPrefix(key, *params.Path) {
			outParameters = append(outParameters, types.Parameter{
				Name:  aws.String(key),
				Value: aws.String(value.(string)),
			})
		}
	}

	return &ssm.GetParametersByPathOutput{
		Parameters: outParameters,
	}, nil
}

func (c *ssmMockClient) GetParameters(_ context.Context, params *ssm.GetParametersInput, _ ...func(*ssm.Options)) (*ssm.GetParametersOutput, error) {
	outParameters := []types.Parameter{}
	for key, value := range c.parameters {
		for _, name := range params.Names {
			if key == name {
				outParameters = append(outParameters, types.Parameter{
					Name:  aws.String(key),
					Value: aws.String(value.(string)),
				})
				break
			}
		}
	}

	return &ssm.GetParametersOutput{
		Parameters: outParameters,
	}, nil
}

func (c *ssmMockClient) GetParameter(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if params == nil || params.Name == nil {
		return nil, nil
	}

	paramName := *params.Name
	if value, exists := c.parameters[paramName]; exists {
		return &ssm.GetParameterOutput{
			Parameter: &types.Parameter{
				Name:  aws.String(paramName),
				Value: aws.String(value.(string)),
			},
		}, nil
	}

	// Return AWS-like error for parameter not found
	return nil, &types.ParameterNotFound{
		Message: aws.String("Parameter " + paramName + " not found."),
	}
}

func TestSSMParameterStoreBackend_ParametersByPath(t *testing.T) {
	mockClient := &ssmMockClient{
		parameters: map[string]interface{}{
			"/group1/key1":      "value1",
			"/group1/nest/key2": "value2",
			"/group2/key3":      "value3",
		},
	}
	getSSMClient = func(_ aws.Config) ssmClient {
		return mockClient
	}

	ssmParameterStoreBackendParams := map[string]interface{}{
		"backend_type": "aws.ssm",
	}
	ssmParameterStoreSecretsBackend, err := NewSSMParameterStoreBackend(ssmParameterStoreBackendParams)
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
