// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package aws allows to fetch secrets from Aws SSM and Secrets Manager service
package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/mitchellh/mapstructure"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/internal/awsutil"
	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
)

// secretsManagerClient is an interface for fetching secrets from AWS Secrets Manager.
// Tests provide a mock implementation.
type secretsManagerClient interface {
	GetSecretValue(ctx context.Context, secretID string) (*string, error)
}

// getSecretsManagerClient is a variable that holds the function to create a new secretsManagerClient.
// It is overwritten in tests.
var getSecretsManagerClient = func(cfg aws.Config) secretsManagerClient {
	return &smHTTPClient{cfg: cfg}
}

// SecretsManagerBackendConfig is the configuration for a AWS Secret Manager backend
type SecretsManagerBackendConfig struct {
	Session     SessionBackendConfig `mapstructure:"aws_session"`
	ForceString bool                 `mapstructure:"force_string"`
}

// SecretsManagerBackend represents backend for AWS Secret Manager
type SecretsManagerBackend struct {
	Config SecretsManagerBackendConfig
	Client secretsManagerClient
}

// NewSecretsManagerBackend returns a new AWS Secret Manager backend
func NewSecretsManagerBackend(bc map[string]interface{}) (
	*SecretsManagerBackend, error) {

	backendConfig := SecretsManagerBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	cfg, err := newConfigFromBackendConfig(backendConfig.Session)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize aws session: %s", err)
	}
	client := getSecretsManagerClient(*cfg)

	backend := &SecretsManagerBackend{
		Config: backendConfig,
		Client: client,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *SecretsManagerBackend) GetSecretOutput(ctx context.Context, secretString string) secret.Output {
	segments := strings.SplitN(secretString, ";", 2)
	if len(segments) != 2 {
		es := "invalid secret format, expected 'secret_id;key'"
		return secret.Output{Value: nil, Error: &es}
	}
	secretID := segments[0]
	secretKey := segments[1]

	secretValue, err := b.Client.GetSecretValue(ctx, secretID)
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	if secretValue == nil {
		es := "secret string is nil"
		return secret.Output{Value: nil, Error: &es}
	}

	var value string
	if b.Config.ForceString {
		value = *secretValue
	} else {
		decoder := json.NewDecoder(strings.NewReader(*secretValue))
		decoder.UseNumber()
		// Try to parse as JSON first
		var jsonSecrets map[string]interface{}
		if err := decoder.Decode(&jsonSecrets); err != nil {
			// If JSON parsing fails, treat the entire string as the value
			value = *secretValue
		} else {
			// If JSON parsing succeeds, look for the specific key
			if val, ok := jsonSecrets[secretKey]; ok {
				switch v := val.(type) {
				case string:
					value = v
				case json.Number:
					value = v.String()
				case map[string]interface{}, []interface{}:
					if b, err := json.Marshal(v); err == nil {
						value = string(b)
					} else {
						value = fmt.Sprintf("%v", v)
					}
				default:
					value = fmt.Sprintf("%v", v)
				}
			} else {
				es := secret.ErrKeyNotFound.Error()
				return secret.Output{Value: nil, Error: &es}
			}
		}
	}

	return secret.Output{Value: &value, Error: nil}
}

// --- Raw HTTP implementation of Secrets Manager ---

type smHTTPClient struct {
	cfg aws.Config
}

type smGetSecretValueResponse struct {
	SecretString *string `json:"SecretString"`
}

func (c *smHTTPClient) GetSecretValue(ctx context.Context, secretID string) (*string, error) {
	region := c.cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	endpoint := awsutil.ServiceEndpoint("secretsmanager", region)

	payload := fmt.Sprintf(`{"SecretId":%q}`, secretID)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "secretsmanager.GetSecretValue")

	creds, err := c.cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS credentials: %w", err)
	}
	awsutil.SignRequest(req, creds, region, "secretsmanager", []byte(payload))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SecretsManager GetSecretValue returned %d: %s", resp.StatusCode, string(body))
	}

	var result smGetSecretValueResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse SecretsManager response: %w", err)
	}

	return result.SecretString, nil
}
