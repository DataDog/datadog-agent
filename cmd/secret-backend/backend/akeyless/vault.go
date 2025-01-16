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
	"net/http"

	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"
)

// BackendConfig is the configuration for a akeyless backend
type BackendConfig struct {
	AkeylessSession SessionBackendConfig `mapstructure:"akeyless_session"`
	BackendType     string               `mapstructure:"backend_type"`
	AkeylessURL     string               `mapstructure:"akeyless_url"`
}

// Backend represents backend for Akeyless
type Backend struct {
	BackendID string
	Config    BackendConfig
	Token     string
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

// NewAkeylessBackend returns a new Akeyless backend
func NewAkeylessBackend(backendID string, bc map[string]interface{}) (*Backend, error) {
	backendConfig := BackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendID).
			Msg("failed to map backend configuration")
		return nil, err
	}

	authToken, err := NewAkeylessConfigFromBackendConfig(backendConfig.AkeylessURL, backendConfig.AkeylessSession)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendID).
			Msg("failed to initialize Akeyless session")
		return nil, err
	}

	backend := &Backend{
		BackendID: backendID,
		Config:    backendConfig,
		Token:     authToken,
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
		log.Error().
			Str("backend_id", b.BackendID).
			Str("backend_type", b.Config.BackendType).
			Str("secret_key", secretKey).
			Msg("failed to marshal payload")
		return secret.Output{Value: nil, Error: &es}
	}

	// Prepare the request
	req, err := http.NewRequest("POST", b.Config.AkeylessURL+"/get-secret-value", bytes.NewBuffer(requestPayload))
	if err != nil {
		es := err.Error()
		log.Error().
			Str("backend_id", b.BackendID).
			Str("backend_type", b.Config.BackendType).
			Str("secret_key", secretKey).
			Msg("failed to create request")
		return secret.Output{Value: nil, Error: &es}
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		es := err.Error()
		log.Error().
			Str("backend_id", b.BackendID).
			Str("backend_type", b.Config.BackendType).
			Str("secret_key", secretKey).
			Msg("failed to send request")
		return secret.Output{Value: nil, Error: &es}
	}
	defer resp.Body.Close()

	// Dump the response for debugging
	//respDump, err := httputil.DumpResponse(resp, true)
	//if err != nil {
	//	log.Error().
	//		Str("backend_id", b.BackendID).
	//		Str("backend_type", b.Config.BackendType).
	//		Str("secret_key", secretKey).
	//		Msg("failed to dump response")
	//} else {
	//	log.Info().
	//		Str("backend_id", b.BackendID).
	//		Str("backend_type", b.Config.BackendType).
	//		Str("secret_key", secretKey).
	//		Msgf("Response:\n%s", string(respDump))
	//}

	// Handle the response
	var response secretResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		es := err.Error()
		log.Error().
			Str("backend_id", b.BackendID).
			Str("backend_type", b.Config.BackendType).
			Str("secret_key", secretKey).
			Msg("failed to decode response")
		return secret.Output{Value: nil, Error: &es}
	}

	// Extract the secret value from the response
	secretValue, ok := response[secretKey]
	if !ok {
		es := secret.ErrKeyNotFound.Error()
		log.Error().
			Str("backend_id", b.BackendID).
			Str("backend_type", b.Config.BackendType).
			Str("secret_key", secretKey).
			Msg("failed to retrieve secret from response")
		return secret.Output{Value: nil, Error: &es}
	}

	return secret.Output{Value: &secretValue, Error: nil}
}
