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
	"net/url"
	"regexp"
	"strings"
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
// Examples: https://agent.datad0g.com., http://metrics.us1.datadoghq.com, agent.ddog-gov.com,
// trace.agent.datadoghq.com, agent-http-intake.logs.us3.datadoghq.com (multi-label subdomains).
// The subdomain group is lazy and optional (*?) so a regional prefix like "us3." is captured on its
// own rather than swallowed as part of the subdomain, and a bare site like "us5.datadoghq.com" or
// "datadoghq.com" (no leading subdomain) still matches.
var domainURLRegexp = regexp.MustCompile(`^(?:https?://)?(?:[^./]+\.)*?((?:[a-z]{2,}\d{1,2}\.)?)(?:(datadoghq|datad0g)\.(com|eu)|(ddog-gov\.com))(\.)?\/?$`)

// hostOnly extracts just the host from endpoint (which may be a bare hostname or a full URL,
// e.g. apm_config.profiling_additional_endpoints uses full URLs as map keys) so domainURLRegexp,
// which expects a bare hostname, can match it. Returns endpoint unchanged if it can't be parsed.
func hostOnly(endpoint string) string {
	raw := endpoint
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return endpoint
	}
	// Hostname() strips a port if present (e.g. "agent.datadoghq.com:443" -> "agent.datadoghq.com")
	// - domainURLRegexp expects a bare hostname and doesn't account for a trailing port.
	return strings.ToLower(u.Hostname())
}

// getAPIDomain transforms intake/metrics endpoints (e.g., agent.datad0g.com) to API endpoints (e.g., api.datad0g.com)
// for known Datadog domains. This ensures API operations use the correct subdomain. endpoint may be
// a bare hostname, a full URL, or a full URL with a path - only the host is matched against the
// known Datadog domain pattern.
// If the endpoint doesn't match a known Datadog domain pattern, it is returned unchanged. When
// warnIfUnknown is true, a warning is logged, since the caller (GetAPIKey, via resolveTokenURL) is
// about to POST a signed cloud auth proof to this arbitrary host, which could be replayed by
// whoever controls it. warnIfUnknown should be false for the agent's own primary site fallback
// (e.g. a supported HTTP proxy dd_url), which is expected to legitimately not match this pattern.
func getAPIDomain(endpoint string, warnIfUnknown bool) string {
	matches := domainURLRegexp.FindStringSubmatch(hostOnly(endpoint))
	if matches == nil {
		// Not a known Datadog domain pattern - this could be a custom endpoint (e.g. a proxy) or an
		// unexpected format.
		if warnIfUnknown {
			log.Warnf("Delegated auth target '%s' is not a recognized Datadog domain; sending the signed cloud auth proof to it unchanged", endpoint)
		} else {
			log.Debugf("Endpoint '%s' does not match known Datadog domain pattern, using unchanged", endpoint)
		}
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

// resolveTokenURL builds the intake-key exchange URL for a given targetSite, falling back to the
// agent's configured primary site when targetSite is empty.
func resolveTokenURL(cfg pkgconfigmodel.Reader, targetSite string) string {
	site := targetSite
	isDelegatedAuthTarget := site != ""
	if site == "" {
		site = utils.GetInfraEndpoint(cfg)
	}
	// Transform the endpoint to use the API subdomain (api.*). Only warn when this is an explicit
	// delegated-auth target site, not the agent's own primary site fallback - a supported HTTP
	// proxy dd_url legitimately won't match the known-Datadog-domain pattern either.
	site = getAPIDomain(site, isDelegatedAuthTarget)
	return fmt.Sprintf(tokenURLEndpoint, site)
}

// GetAPIKey performs the cloud auth exchange and returns an API key.
// The delegatedAuthProof contains the signed AWS request which includes the org id.
//
// targetSite, if non-empty, is the site/domain to exchange the proof against (e.g. an
// `additional_endpoints` domain for a dual-shipping DELA(...) instance targeting a different
// site than the agent's primary `dd_url`/`site`). If empty, falls back to the agent's configured
// primary site - the original, single-org behavior.
func GetAPIKey(cfg pkgconfigmodel.Reader, delegatedAuthProof string, targetSite string) (*string, error) {
	var apiKey *string

	url := resolveTokenURL(cfg, targetSite)
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
		// Surface the server's JSON:API error detail (ex: the verified identity + "no identity
		// mapping was found" guidance from the intake-key endpoint), which is otherwise only
		// visible in Datadog's internal logs. We extract only the server-authored title/detail
		// fields, never the raw body, so a successful-path token (200 only) or any unexpected
		// content cannot leak into logs.
		err = fmt.Errorf("failed to get API key: %s%s", resp.Status, errorDetail(tokenBytes))
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

// errorDetail extracts the human-readable message(s) from a JSON:API error response body,
// returning ": <detail>" for appending to an error. It surfaces only the server-authored
// title/detail fields (never the raw body), so a successful-path token or any unexpected content
// cannot leak into logs. Returns "" when the body is not a JSON:API error envelope.
func errorDetail(body []byte) string {
	var envelope struct {
		Errors []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	var msgs []string
	for _, e := range envelope.Errors {
		switch {
		case e.Detail != "":
			msgs = append(msgs, e.Detail)
		case e.Title != "":
			msgs = append(msgs, e.Title)
		}
	}
	if len(msgs) == 0 {
		return ""
	}
	return ": " + strings.Join(msgs, "; ")
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
