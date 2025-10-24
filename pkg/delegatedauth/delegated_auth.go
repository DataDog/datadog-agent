// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package delegatedauth provides the configuration and implementation for exchanging an delegated auth proof for an API key
package delegatedauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tokenURLEndpoint  = "%s/api/v2/intake-key"
	authorizationType = "Delegated"

	contentTypeHeader   = "Content-Type"
	authorizationHeader = "Authorization"
	applicationJSON     = "application/json"
)

func getSite(cfg pkgconfigmodel.Reader) string {
	site := pkgconfigsetup.DefaultSite
	if cfg.GetString("site") != "" {
		site = cfg.GetString("site")
	}

	return utils.BuildURLWithPrefix("https://", site)
}

// GetApiKey actually performs the cloud auth exchange and returns an API key. It is called be each individual provider
func GetApiKey(cfg pkgconfigmodel.Reader, orgUUID, delegatedAuthProof string) (*string, error) {
	site := getSite(cfg)
	var apiKey *string

	log.Infof("Fetching api key for site %s", site)
	url := fmt.Sprintf(tokenURLEndpoint, site)

	transport := httputils.CreateHTTPTransport(cfg)
	client := &http.Client{
		Transport: transport,
	}
	log.Infof("Getting api key from: %s with cloud auth proof", url)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer([]byte("")))
	if err != nil {
		return nil, err
	}
	req.Header.Set(contentTypeHeader, applicationJSON)
	req.Header.Set(authorizationHeader, fmt.Sprintf("%s %s", authorizationType, delegatedAuthProof))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	tokenBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("failed to get API key: %s", resp.Status)
		return nil, err
	} else {
		apiKey, err = parseResponse(tokenBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		log.Infof("Successfully parsed delegated API key")
	}
	return apiKey, nil
}

// TokenResponse represents the response from the intake-key endpoint
type TokenResponse struct {
	Data TokenData `json:"data"`
}

// TokenData represents the data field in the token response
type TokenData struct {
	Attributes TokenAttributes `json:"attributes"`
}

// TokenAttributes represents the attributes field containing the API key
type TokenAttributes struct {
	APIKey string `json:"api_key"`
}

func parseResponse(tokenBytes []byte) (*string, error) {
	var tokenResponse TokenResponse
	err := json.Unmarshal(tokenBytes, &tokenResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Validate that we got an API key
	if tokenResponse.Data.Attributes.APIKey == "" {
		return nil, fmt.Errorf("api_key is empty in response")
	}

	return &tokenResponse.Data.Attributes.APIKey, nil
}
