// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package akeyless allows to fetch secrets from akeyless service
package akeyless

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/mitchellh/mapstructure"
)

// BackendConfig is the configuration for a akeyless backend
type BackendConfig struct {
	AkeylessSession SessionBackendConfig `mapstructure:"akeyless_session"`
	AkeylessURL     string               `mapstructure:"akeyless_url"`
}

// Backend represents backend for Akeyless
type Backend struct {
	Config BackendConfig
	Token  string
}

type secretRequest struct {
	AccessID      string   `json:"access-id"`
	AccessKey     string   `json:"access-key"`
	AccessType    string   `json:"access-type"`
	Accessibility string   `json:"accessibility"`
	IgnoreCache   string   `json:"ignore-cache"`
	JSON          bool     `json:"json"`
	Names         []string `json:"names"`
	Token         string   `json:"token"`
}

type secretResponse map[string]string

// SessionBackendConfig is the session configuration for Akeyless
type SessionBackendConfig struct {
	AkeylessAccessID  string `mapstructure:"akeyless_access_id"`
	AkeylessAccessKey string `mapstructure:"akeyless_access_key"`
}

type authRequest struct {
	AccessID   string `json:"access-id"`
	AccessKey  string `json:"access-key"`
	AccessType string `json:"access-type"`
}

type authResponse struct {
	Token string `json:"token"`
	//	might need to add creds
}

// newAkeylessConfigFromBackendConfig returns a new config for Akeyless
func newAkeylessConfigFromBackendConfig(akeylessURL string, sessionConfig SessionBackendConfig) (string, error) {
	requestBody, _ := json.Marshal(authRequest{
		AccessID:   sessionConfig.AkeylessAccessID,
		AccessKey:  sessionConfig.AkeylessAccessKey,
		AccessType: "access_key",
	})

	resp, err := http.Post(strings.TrimRight(akeylessURL, "/")+"/auth", "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", errors.New("failed to authenticate with akeyless")
	}

	defer resp.Body.Close()

	var authResp authResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", err
	}

	return authResp.Token, err
}

// NewAkeylessBackend returns a new Akeyless backend
func NewAkeylessBackend(bc map[string]interface{}) (*Backend, error) {
	backendConfig := BackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	authToken, err := newAkeylessConfigFromBackendConfig(backendConfig.AkeylessURL, backendConfig.AkeylessSession)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Akeyless session: %s", err)
	}

	backend := &Backend{
		Config: backendConfig,
		Token:  authToken,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *Backend) GetSecretOutput(secretKey string) secret.Output {

	payload := secretRequest{
		AccessType:    "access_key",
		Accessibility: "regular",
		IgnoreCache:   "false",
		JSON:          true,
		Names:         []string{secretKey},
		Token:         b.Token,
	}

	// Marshal the payload
	requestPayload, err := json.Marshal(payload)
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	// Prepare the request
	req, err := http.NewRequest("POST", b.Config.AkeylessURL+"/get-secret-value", bytes.NewBuffer(requestPayload))
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}
	defer resp.Body.Close()

	// Handle the response
	var response secretResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	// Extract the secret value from the response
	secretValue, ok := response[secretKey]
	if !ok {
		es := secret.ErrKeyNotFound.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	return secret.Output{Value: &secretValue, Error: nil}
}
