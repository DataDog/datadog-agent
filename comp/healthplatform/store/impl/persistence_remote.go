// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package storeimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	remoteIssuesEndpointPrefix = "https://api."
	remoteIssuesEndpointPath   = "/api/v2/agenthealth/hosts/%s/issues"
	remoteIssuesResourceType   = "agent_health_issue"
	remoteIssuesHTTPTimeout    = 10 * time.Second
	remoteIssuesMaxResponse    = 10 * 1024 * 1024
	jsonAPIContentType         = "application/vnd.api+json"
)

type remotePersistence struct {
	config     config.Component
	hostname   hostnameinterface.Component
	baseURL    string
	httpClient *http.Client
}

type remoteIssuesResponse struct {
	Data *[]remoteIssueResource `json:"data"`
}

type remoteIssueResource struct {
	ID         string                `json:"id"`
	Type       string                `json:"type"`
	Attributes remoteIssueAttributes `json:"attributes"`
}

type remoteIssueAttributes struct {
	IssueName  string `json:"issue_name"`
	DetectedAt string `json:"detected_at"`
}

func newRemotePersistence(cfg config.Component, hostname hostnameinterface.Component) *remotePersistence {
	site := strings.TrimSpace(cfg.GetString("site"))
	if site == "" {
		site = constants.DefaultSite
	}
	return &remotePersistence{
		config:   cfg,
		hostname: hostname,
		baseURL:  configutils.BuildURLWithPrefix(remoteIssuesEndpointPrefix, site),
		httpClient: &http.Client{
			Timeout:       remoteIssuesHTTPTimeout,
			Transport:     httputils.CreateHTTPTransport(cfg),
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func hasRemotePersistenceCredentials(cfg config.Component) bool {
	return configutils.SanitizeAPIKey(cfg.GetString("api_key")) != "" &&
		configutils.SanitizeAPIKey(cfg.GetString("app_key")) != ""
}

func (r *remotePersistence) load(ctx context.Context) (*PersistedState, error) {
	apiKey := configutils.SanitizeAPIKey(r.config.GetString("api_key"))
	appKey := configutils.SanitizeAPIKey(r.config.GetString("app_key"))
	if apiKey == "" || appKey == "" {
		return nil, errors.New("API key and application key are required for remote issue persistence")
	}

	hostname, err := r.hostname.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve hostname for remote issue persistence: %w", err)
	}
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return nil, errors.New("hostname is required for remote issue persistence")
	}

	endpoint := strings.TrimRight(r.baseURL, "/") + fmt.Sprintf(remoteIssuesEndpointPath, url.PathEscape(hostname))
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse remote issue endpoint: %w", err)
	}
	if !strings.EqualFold(endpointURL.Scheme, "https") {
		return nil, errors.New("remote issue endpoint must use HTTPS")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create remote issue request: %w", err)
	}
	req.Header.Set("Accept", jsonAPIContentType)
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("DD-APPLICATION-KEY", appKey)
	req.Header.Set("DD-Agent-Version", version.AgentVersion)
	req.Header.Set("User-Agent", "datadog-agent/"+version.AgentVersion)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("load remote issues: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, remoteIssuesMaxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read remote issue response: %w", err)
	}
	if len(body) > remoteIssuesMaxResponse {
		return nil, fmt.Errorf("remote issue response exceeds %d bytes", remoteIssuesMaxResponse)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote issue request returned status %d", resp.StatusCode)
	}

	var response remoteIssuesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode remote issue response: %w", err)
	}
	if response.Data == nil {
		return nil, errors.New("remote issue response must contain a data array")
	}
	resources := *response.Data

	now := time.Now().UTC().Format(time.RFC3339)
	state := &PersistedState{
		Version:   persistedStateVersion,
		UpdatedAt: now,
		Issues:    make(map[string]*PersistedIssue, len(resources)),
	}
	for i, resource := range resources {
		if resource.Type != remoteIssuesResourceType {
			return nil, fmt.Errorf("remote issue at index %d has unexpected type %q", i, resource.Type)
		}
		if resource.ID == "" {
			return nil, fmt.Errorf("remote issue at index %d has no ID", i)
		}
		if resource.Attributes.IssueName == "" {
			return nil, fmt.Errorf("remote issue %q has no issue name", resource.ID)
		}
		if _, exists := state.Issues[resource.ID]; exists {
			// Distinct recommendations can represent the same issue. The store is
			// keyed by issue ID, so retain the first occurrence.
			continue
		}

		firstSeen := resource.Attributes.DetectedAt
		if firstSeen == "" {
			firstSeen = now
		}

		state.Issues[resource.ID] = &PersistedIssue{
			IssueID:   resource.ID,
			IssueType: resource.Attributes.IssueName,
			State:     IssueStateActive,
			FirstSeen: firstSeen,
			LastSeen:  firstSeen,
		}
	}

	return state, nil
}

func (r *remotePersistence) save(_ *PersistedState) error { return nil }
