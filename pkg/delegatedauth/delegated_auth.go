// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package delegatedauth provides the configuration and implementation for exchanging an delegated auth proof for an API key
package delegatedauth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
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

// domainURLRegexp matches and captures known Datadog domains with optional protocol and trailing characters
// Captures: protocol (optional), subdomain (ignored), regional prefix + base domain, trailing dot (optional)
// Examples: https://agent.datad0g.com., http://metrics.us1.datadoghq.com, agent.ddog-gov.com
var domainURLRegexp = regexp.MustCompile(`^(?:https?://)?[^./]+\.((?:[a-z]{2,}\d{1,2}\.)?)(?:(datadoghq|datad0g)\.(com|eu)|(ddog-gov\.com))(\.)?\/?$`)

// getAPIDomain transforms intake/metrics endpoints (e.g., agent.datad0g.com) to API endpoints (e.g., app.datad0g.com)
// for known Datadog domains. This ensures API operations use the correct subdomain.
func getAPIDomain(endpoint string) string {
	matches := domainURLRegexp.FindStringSubmatch(endpoint)
	if matches == nil {
		// Not a known Datadog domain, return unchanged
		return endpoint
	}

	// matches[1] = regional prefix (e.g., "us1.", "eu1.", or "")
	// matches[2] = base domain name (e.g., "datadoghq", "datad0g", or "")
	// matches[3] = TLD (e.g., "com", "eu", or "")
	// matches[4] = gov cloud domain (e.g., "ddog-gov.com", or "")
	// matches[5] = trailing dot (e.g., ".", or "")

	var baseDomain string
	if matches[4] != "" {
		// Gov cloud domain
		baseDomain = matches[4]
	} else {
		// Regular Datadog domain
		baseDomain = matches[1] + matches[2] + "." + matches[3]
	}

	// Append trailing dot if present
	if matches[5] != "" {
		baseDomain += "."
	}

	return "https://api." + baseDomain
}

// GetAPIKey actually performs the cloud auth exchange and returns an API key. It is called be each individual provider
func GetAPIKey(cfg pkgconfigmodel.Reader, _, delegatedAuthProof string) (*string, error) {
	var apiKey *string

	site := utils.GetInfraEndpoint(cfg)
	// Transform the endpoint to use the API subdomain (api.*)
	site = getAPIDomain(site)
	url := fmt.Sprintf(tokenURLEndpoint, site)
	log.Infof("Fetching api key for url %s", url)

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
	defer resp.Body.Close()

	tokenBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("failed to get API key: %s", resp.Status)
		return nil, err
	}

	apiKey, err = parseResponse(tokenBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	log.Infof("Successfully parsed delegated API key")
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
		return nil, errors.New("api_key is empty in response")
	}

	return &tokenResponse.Data.Attributes.APIKey, nil
}
