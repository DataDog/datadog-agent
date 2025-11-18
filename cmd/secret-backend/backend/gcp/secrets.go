// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package gcp allows to fetch secrets from GCP Secret Manager service
package gcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const secretManagerScope = "https://www.googleapis.com/auth/cloud-platform"

var serviceEndpoint = "https://secretmanager.googleapis.com/v1"

// SecretManagerBackendConfig is the configuration for GCP Secret Manager backend
type SecretManagerBackendConfig struct {
	Session struct {
		ProjectID string `mapstructure:"project_id"`
	} `mapstructure:"gcp_session"`
}

// SecretManagerBackend represents backend for GCP Secret Manager
type SecretManagerBackend struct {
	Config SecretManagerBackendConfig
	Client *http.Client
}

// https://docs.cloud.google.com/secret-manager/docs/reference/rest/v1/AccessSecretVersionResponse
type accessSecretVersionResponse struct {
	Name    string `json:"name"`
	Payload struct {
		Data       string `json:"data"`
		DataCRC32C string `json:"dataCrc32c"`
	} `json:"payload"`
}

// NewSecretManagerBackend returns a new GCP Secret Manager backend
func NewSecretManagerBackend(bc map[string]interface{}) (*SecretManagerBackend, error) {
	backendConfig := SecretManagerBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	if backendConfig.Session.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required in gcp_session configuration")
	}

	// use application default credentials (ADC) to authenticate
	ctx := context.Background()
	credentials, err := google.FindDefaultCredentials(ctx, secretManagerScope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default credentials: %v", err)
	}

	// oauth2.NewClient automatically adds the Bearer token to all requests
	return &SecretManagerBackend{
		Config: backendConfig,
		Client: oauth2.NewClient(ctx, credentials.TokenSource),
	}, nil
}

// GetSecretOutput retrieves a secret from GCP Secret Manager
func (b *SecretManagerBackend) GetSecretOutput(ctx context.Context, secretString string) secret.Output {
	// parse: secret[;key] or secret;version;[key]

	parts := strings.Split(secretString, ";")
	secretName, secretVersion, jsonKey := parts[0], "latest", ""
	switch {
	case len(parts) >= 3:
		secretVersion, jsonKey = parts[1], parts[2]
		if secretVersion == "" {
			secretVersion = "latest"
		}
	case len(parts) == 2:
		jsonKey = parts[1]
	}

	// https://secretmanager.googleapis.com/v1/projects/{project}/secrets/{secret}/versions/{version}:access
	url := fmt.Sprintf("%s/projects/%s/secrets/%s/versions/%s:access",
		serviceEndpoint, b.Config.Session.ProjectID, secretName, secretVersion)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		e := fmt.Sprintf("failed to create request: %v", err)
		return secret.Output{Value: nil, Error: &e}
	}

	resp, err := b.Client.Do(req)
	if err != nil {
		e := fmt.Sprintf("failed to access secret: %v", err)
		return secret.Output{Value: nil, Error: &e}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		e := fmt.Sprintf("API error (status %d)", resp.StatusCode)
		return secret.Output{Value: nil, Error: &e}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e := fmt.Sprintf("failed to read response: %v", err)
		return secret.Output{Value: nil, Error: &e}
	}

	var result accessSecretVersionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		e := fmt.Sprintf("failed to parse response: %v", err)
		return secret.Output{Value: nil, Error: &e}
	}

	// payload data is base64-encoded
	decoded, err := base64.StdEncoding.DecodeString(result.Payload.Data)
	if err != nil {
		e := fmt.Sprintf("failed to decode secret data: %v", err)
		return secret.Output{Value: nil, Error: &e}
	}

	value := string(decoded)
	if jsonKey == "" {
		return secret.Output{Value: &value, Error: nil}
	}

	var secretValue map[string]string
	if json.Unmarshal(decoded, &secretValue) == nil {
		if val, ok := secretValue[jsonKey]; ok {
			return secret.Output{Value: &val, Error: nil}
		}
	}

	e := fmt.Sprintf("key '%s' not found in secret JSON", jsonKey)
	return secret.Output{Value: nil, Error: &e}
}
