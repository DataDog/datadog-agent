// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package api provides the configuration and implementation for exchanging a delegated auth proof for an API key
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

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

	// httpClientTimeout is the maximum time to wait for the HTTP request to complete
	httpClientTimeout = 30 * time.Second

	// maxResponseBodySize limits the response body to prevent memory exhaustion (1 MB should be more than enough for an API key response)
	maxResponseBodySize = 1 * 1024 * 1024
)

// domainURLRegexp matches and captures known Datadog domains with optional protocol and trailing characters
// Captures: protocol (optional), subdomain (ignored), regional prefix + base domain, trailing dot (optional)
// Examples: https://agent.datad0g.com., http://metrics.us1.datadoghq.com, agent.ddog-gov.com
var domainURLRegexp = regexp.MustCompile(`^(?:https?://)?[^./]+\.((?:[a-z]{2,}\d{1,2}\.)?)(?:(datadoghq|datad0g)\.(com|eu)|(ddog-gov\.com))(\.)?\/?$`)

// getAPIDomain transforms intake/metrics endpoints (e.g., agent.datad0g.com) to API endpoints (e.g., app.datad0g.com)
// for known Datadog domains. This ensures API operations use the correct subdomain.
// If the endpoint doesn't match a known Datadog domain pattern, it is returned unchanged with a debug log.
func getAPIDomain(endpoint string) string {
	matches := domainURLRegexp.FindStringSubmatch(endpoint)
	if matches == nil {
		// Not a known Datadog domain pattern - this could be a custom endpoint or unexpected format
		log.Debugf("Endpoint '%s' does not match known Datadog domain pattern, using unchanged", endpoint)
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

// GetAPIKey performs the cloud auth exchange and returns an API key.
// The delegatedAuthProof contains the signed AWS request which includes the org id.
func GetAPIKey(cfg pkgconfigmodel.Reader, delegatedAuthProof string) (*string, error) {
	var apiKey *string

	site := utils.GetInfraEndpoint(cfg)
	// Transform the endpoint to use the API subdomain (api.*)
	site = getAPIDomain(site)
	url := fmt.Sprintf(tokenURLEndpoint, site)
	log.Infof("Getting API key from: %s with cloud auth proof", url)

	transport := httputils.CreateHTTPTransport(cfg)
	client := &http.Client{
		Transport: transport,
		Timeout:   httpClientTimeout,
	}
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

	// Limit response body size to prevent memory exhaustion from malicious/malformed responses
	limitedReader := io.LimitReader(resp.Body, maxResponseBodySize)
	tokenBytes, err := io.ReadAll(limitedReader)
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
