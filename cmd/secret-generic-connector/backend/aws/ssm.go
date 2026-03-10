// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mitchellh/mapstructure"

	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/internal/awsutil"
	"github.com/DataDog/datadog-agent/cmd/secret-generic-connector/secret"
)

// ssmClient is an interface that defines the methods we use from the SSM client.
// Tests provide a mock implementation.
type ssmClient interface {
	GetParameter(ctx context.Context, name string, withDecryption bool) (value *string, err error)
}

// getSSMClient is a variable that holds the function to create a new ssmClient.
// It is overwritten in tests.
var getSSMClient = func(cfg *awsutil.AWSConfig) ssmClient {
	return &ssmHTTPClient{cfg: cfg}
}

// SSMParameterStoreBackendConfig is the configuration for a AWS SSM backend
type SSMParameterStoreBackendConfig struct {
	Session SessionBackendConfig `mapstructure:"aws_session"`
}

// SSMParameterStoreBackend represents backend for AWS SSM
type SSMParameterStoreBackend struct {
	Config SSMParameterStoreBackendConfig
	Client ssmClient
}

// NewSSMParameterStoreBackend returns a new AWS SSM backend
func NewSSMParameterStoreBackend(bc map[string]interface{}) (*SSMParameterStoreBackend, error) {
	backendConfig := SSMParameterStoreBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	cfg, err := newConfigFromBackendConfig(backendConfig.Session)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize aws session: %s", err)
	}
	client := getSSMClient(cfg)

	backend := &SSMParameterStoreBackend{
		Config: backendConfig,
		Client: client,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *SSMParameterStoreBackend) GetSecretOutput(ctx context.Context, secretKey string) secret.Output {
	value, err := b.Client.GetParameter(ctx, secretKey, true)
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	if value == nil {
		es := "parameter value is nil"
		return secret.Output{Value: nil, Error: &es}
	}

	return secret.Output{Value: value, Error: nil}
}

// --- Raw HTTP implementation of SSM ---

type ssmHTTPClient struct {
	cfg *awsutil.AWSConfig
}

type ssmGetParameterRequest struct {
	Name           string `json:"Name"`
	WithDecryption bool   `json:"WithDecryption"`
}

type ssmGetParameterResponse struct {
	Parameter *ssmParameter `json:"Parameter"`
}

type ssmParameter struct {
	Name  *string `json:"Name"`
	Value *string `json:"Value"`
}

func (c *ssmHTTPClient) GetParameter(ctx context.Context, name string, withDecryption bool) (*string, error) {
	if c.cfg.Region == "" {
		return nil, fmt.Errorf("AWS region is required for SSM")
	}

	endpoint := awsutil.ServiceEndpoint("ssm", c.cfg.Region)

	reqBody, err := json.Marshal(ssmGetParameterRequest{
		Name:           name,
		WithDecryption: withDecryption,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonSSM.GetParameter")

	awsutil.SignRequest(req, c.cfg.Credentials, c.cfg.Region, "ssm", reqBody)

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
		return nil, fmt.Errorf("SSM GetParameter returned %d: %s", resp.StatusCode, string(body))
	}

	var result ssmGetParameterResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse SSM response: %w", err)
	}

	if result.Parameter == nil || result.Parameter.Value == nil {
		return nil, nil
	}

	return result.Parameter.Value, nil
}
