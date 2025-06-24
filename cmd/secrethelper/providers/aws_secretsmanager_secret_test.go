package providers

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
)

// secretsManagerMockClient is the struct we'll use to mock the Secrets Manager client
// for unit tests. E2E tests should be written with the real client.
type secretsManagerMockClient struct {
	secrets map[string]secretsmanager.GetSecretValueOutput
}

func (c *secretsManagerMockClient) GetSecretValue(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if params == nil || params.SecretId == nil {
		return nil, nil
	}

	for key, value := range c.secrets {
		if key == *params.SecretId {
			return &value, nil
		}
	}
	return nil, errors.New("secret not found")
}

func TestReadAwsSecretsManagerSecret(t *testing.T) {
	tests := []struct {
		name            string
		existingSecrets map[string]secretsmanager.GetSecretValueOutput
		secretPath      string
		expectedValue   string
		expectedError   string
	}{
		{
			name:            "invalid arn",
			existingSecrets: map[string]secretsmanager.GetSecretValueOutput{},
			secretPath:      "key 1",
			expectedError:   "Invalid format. Use: \"arn:aws:secretsmanager:<REGION>:<ACCOUNT_ID>:secret:<SECRET_NAME>\"",
		},
		{
			name:            "secret does not exist",
			existingSecrets: map[string]secretsmanager.GetSecretValueOutput{},
			secretPath:      "arn:aws:secretsmanager:us-east-1:000000000000:secret:my/prod/secret",
			expectedError:   "Secrets Manager read error: secret not found",
		},
		{
			name: "secret exists",
			existingSecrets: map[string]secretsmanager.GetSecretValueOutput{
				"arn:aws:secretsmanager:us-east-1:000000000000:secret:key1": {
					Name:         aws.String("key1"),
					SecretString: aws.String("foo bar"),
				},
				"arn:aws:secretsmanager:ca-central-1:000000000000:secret:key2": {
					Name:         aws.String("key2"),
					SecretString: aws.String("bar baz"),
				},
			},
			secretPath:    "arn:aws:secretsmanager:ca-central-1:000000000000:secret:key2",
			expectedValue: "bar baz",
			expectedError: "",
		},
		{
			name: "encoded string",
			existingSecrets: map[string]secretsmanager.GetSecretValueOutput{
				"arn:aws:secretsmanager:us-east-1:000000000000:secret:my/encoded/secret": {
					Name:         aws.String("my/encoded/secret"),
					SecretBinary: []byte(base64.StdEncoding.EncodeToString([]byte("my encoded secret"))),
				},
			},
			secretPath:    "arn:aws:secretsmanager:us-east-1:000000000000:secret:my/encoded/secret",
			expectedValue: "my encoded secret",
			expectedError: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockClient := &secretsManagerMockClient{
				secrets: test.existingSecrets,
			}
			getSecretsManagerClient = func(_ aws.Config) secretsManagerClient {
				return mockClient
			}

			resolvedSecret := ReadAwsSecretsManagerSecret(test.secretPath)

			if test.expectedError != "" {
				assert.Equal(t, test.expectedError, resolvedSecret.ErrorMsg)
			} else {
				assert.Equal(t, test.expectedValue, resolvedSecret.Value)
				assert.Empty(t, test.expectedError, resolvedSecret.ErrorMsg)
			}
		})
	}
}
