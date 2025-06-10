// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package aws

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
)

// secretsManagerMockClient is the struct we'll use to mock the Secrets Manager client
// for unit tests. E2E tests should be written with the real client.
type secretsManagerMockClient struct {
	secrets map[string]interface{}
}

func (c *secretsManagerMockClient) GetSecretValue(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if params == nil || params.SecretId == nil {
		return nil, nil
	}

	for key, value := range c.secrets {
		if key == *params.SecretId {
			return &secretsmanager.GetSecretValueOutput{
				Name:         aws.String(key),
				SecretString: aws.String(value.(string)),
			}, nil
		}
	}
	return nil, secret.ErrKeyNotFound
}

func TestSecretsManagerBackend(t *testing.T) {
	mockClient := &secretsManagerMockClient{
		secrets: map[string]interface{}{
			"key1": "{\"user\":\"foo\",\"password\":\"bar\"}",
			"key2": "{\"foo\":\"bar\"}",
		},
	}
	getSecretsManagerClient = func(_ aws.Config) secretsManagerClient {
		return mockClient
	}

	secretsManagerBackendParams := map[string]interface{}{
		"backend_type": "aws.secrets",
		"secret_id":    "key1",
		"force_string": false,
	}
	secretsManagerSecretsBackend, err := NewSecretsManagerBackend(secretsManagerBackendParams)
	assert.NoError(t, err)

	// Top-level keys are not fetchable
	secretOutput := secretsManagerSecretsBackend.GetSecretOutput("key1")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	secretOutput = secretsManagerSecretsBackend.GetSecretOutput("key2")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	// But the contents under the selected key are
	secretOutput = secretsManagerSecretsBackend.GetSecretOutput("user")
	assert.Equal(t, "foo", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsManagerSecretsBackend.GetSecretOutput("password")
	assert.Equal(t, "bar", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)
}

func TestSecretsManagerBackend_ForceString(t *testing.T) {
	mockClient := &secretsManagerMockClient{
		secrets: map[string]interface{}{
			"key1": "{\"user\":\"foo\",\"password\":\"bar\"}",
			"key2": "{\"foo\":\"bar\"}",
		},
	}
	getSecretsManagerClient = func(_ aws.Config) secretsManagerClient {
		return mockClient
	}

	secretsManagerBackendParams := map[string]interface{}{
		"backend_type": "aws.secrets",
		"secret_id":    "key1",
		"force_string": true,
	}
	secretsManagerSecretsBackend, err := NewSecretsManagerBackend(secretsManagerBackendParams)
	assert.NoError(t, err)

	secretOutput := secretsManagerSecretsBackend.GetSecretOutput("_")
	assert.Equal(t, "{\"user\":\"foo\",\"password\":\"bar\"}", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = secretsManagerSecretsBackend.GetSecretOutput("key1")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}

func TestSecretsManagerBackend_NotJSON(t *testing.T) {
	mockClient := &secretsManagerMockClient{
		secrets: map[string]interface{}{
			"key1": "not json",
			"key2": "foobar",
		},
	}
	getSecretsManagerClient = func(_ aws.Config) secretsManagerClient {
		return mockClient
	}

	secretsManagerBackendParams := map[string]interface{}{
		"backend_type": "aws.secrets",
		"secret_id":    "key1",
		"force_string": false,
	}
	secretsManagerSecretsBackend, err := NewSecretsManagerBackend(secretsManagerBackendParams)
	assert.NoError(t, err)

	// Top-level keys are not fetchable
	secretOutput := secretsManagerSecretsBackend.GetSecretOutput("key1")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	secretOutput = secretsManagerSecretsBackend.GetSecretOutput("key2")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)

	// But the contents under the selected key are
	secretOutput = secretsManagerSecretsBackend.GetSecretOutput("_")
	assert.Equal(t, "not json", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)
}
