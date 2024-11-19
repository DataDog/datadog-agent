// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package akeyless

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

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

// NewAkeylessConfigFromBackendConfig returns a new config for Akeyless
func NewAkeylessConfigFromBackendConfig(akeylessURL string, sessionConfig SessionBackendConfig) (string, error) {
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
