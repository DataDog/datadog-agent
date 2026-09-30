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

	"github.com/twmb/murmur3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	confighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	remoteIssuesEndpointPrefix   = "https://api."
	remoteIssuesEndpointPath     = "/api/v2/agenthealth/hosts/%s/issues"
	remoteIssuesAgentTypeParam   = "agent_type"
	remoteIssuesNodeAgentType    = "node"
	remoteIssuesClusterAgentType = "cluster"
	remoteIssuesResourceType     = "agent_health_issue_ids"
	remoteIssuesHTTPTimeout      = 10 * time.Second
	remoteIssuesMaxResponse      = 10 * 1024 * 1024
	jsonAPIContentType           = "application/vnd.api+json"
)

type remoteIssueLoader struct {
	config     config.Component
	agentType  string
	resourceID func() string
	baseURL    string
	httpClient *http.Client
}

type remoteIssuesResponse struct {
	Data *remoteIssueResource `json:"data"`
}

type remoteIssueResource struct {
	ID         string                `json:"id"`
	Type       string                `json:"type"`
	Attributes remoteIssueAttributes `json:"attributes"`
}

type remoteIssueAttributes struct {
	OrgID    *int64    `json:"org_id"`
	IssueIDs *[]string `json:"issue_ids"`
}

type remoteIssueSnapshot struct {
	orgID    int64
	issueIDs map[string]struct{}
}

type remoteResourceIdentity interface {
	DeploymentID() string
	ClusterID() string
}

func newRemoteIssueLoader(cfg config.Component, agentType string, resourceID func() string) *remoteIssueLoader {
	site := strings.TrimSpace(cfg.GetString("site"))
	return &remoteIssueLoader{
		config:     cfg,
		agentType:  agentType,
		resourceID: resourceID,
		baseURL:    configutils.BuildURLWithPrefix(remoteIssuesEndpointPrefix, site),
		httpClient: &http.Client{
			Timeout:       remoteIssuesHTTPTimeout,
			Transport:     httputils.CreateHTTPTransport(cfg),
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func hasRemoteRestorationCredential(cfg config.Component) bool {
	return configutils.SanitizeAPIKey(cfg.GetString("api_key")) != ""
}

func newRemoteIssueLoaderIfEnabled(reqs Requires, agentFlavor string, identity remoteResourceIdentity) *remoteIssueLoader {
	remoteEnabled := reqs.RemoteRestoration != nil && reqs.RemoteRestoration.Enabled
	if !remoteEnabled || confighelper.IsCLCRunner(reqs.Config) {
		reqs.Log.Info("Running on Kubernetes: remote health platform restoration disabled for this process")
		return nil
	}

	if agentFlavor != flavor.DefaultAgent && agentFlavor != flavor.ClusterAgent {
		reqs.Log.Info("Running on Kubernetes: remote health platform restoration disabled for this process")
		return nil
	}

	if reqs.Config.GetBool("fips.enabled") {
		reqs.Log.Info("Running on Kubernetes: remote health platform restoration is unsupported with the FIPS proxy")
		return nil
	}
	if reqs.Config.GetBool("skip_ssl_validation") {
		reqs.Log.Info("Running on Kubernetes: remote health platform restoration requires TLS certificate verification")
		return nil
	}
	if !hasRemoteRestorationCredential(reqs.Config) {
		reqs.Log.Info("Running on Kubernetes: remote health platform restoration requires api_key")
		return nil
	}

	reqs.Log.Info("Running on Kubernetes: restoring health platform issue state from the Datadog API")
	if agentFlavor == flavor.ClusterAgent {
		return newRemoteIssueLoader(reqs.Config, remoteIssuesClusterAgentType, identity.ClusterID)
	}
	return newRemoteIssueLoader(reqs.Config, remoteIssuesNodeAgentType, identity.DeploymentID)
}

func (r *remoteIssueLoader) load(ctx context.Context) (*remoteIssueSnapshot, error) {
	apiKey := configutils.SanitizeAPIKey(r.config.GetString("api_key"))
	if apiKey == "" {
		return nil, errors.New("API key is required for remote issue restoration")
	}

	resourceID := strings.TrimSpace(r.resourceID())
	if resourceID == "" {
		return nil, errors.New("resource ID is required for remote issue restoration")
	}

	endpoint := strings.TrimRight(r.baseURL, "/") + fmt.Sprintf(remoteIssuesEndpointPath, url.PathEscape(resourceID))
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse remote issue endpoint: %w", err)
	}
	query := endpointURL.Query()
	query.Set(remoteIssuesAgentTypeParam, r.agentType)
	endpointURL.RawQuery = query.Encode()
	if !strings.EqualFold(endpointURL.Scheme, "https") {
		return nil, errors.New("remote issue endpoint must use HTTPS")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create remote issue request: %w", err)
	}
	req.Header.Set("Accept", jsonAPIContentType)
	req.Header.Set("DD-API-KEY", apiKey)
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
		return nil, errors.New("remote issue response must contain data")
	}
	resource := response.Data
	if resource.Type != remoteIssuesResourceType {
		return nil, fmt.Errorf("remote issue response has unexpected type %q", resource.Type)
	}
	if resource.ID != resourceID {
		return nil, fmt.Errorf("remote issue response has resource ID %q, expected %q", resource.ID, resourceID)
	}
	if resource.Attributes.OrgID == nil {
		return nil, errors.New("remote issue response has no org_id")
	}
	if resource.Attributes.IssueIDs == nil {
		return nil, errors.New("remote issue response has no issue_ids")
	}

	snapshot := &remoteIssueSnapshot{
		orgID:    *resource.Attributes.OrgID,
		issueIDs: make(map[string]struct{}, len(*resource.Attributes.IssueIDs)),
	}
	for _, issueID := range *resource.Attributes.IssueIDs {
		if strings.TrimSpace(issueID) == "" {
			return nil, errors.New("remote issue response contains an empty issue ID")
		}
		snapshot.issueIDs[issueID] = struct{}{}
	}

	return snapshot, nil
}

func (s *remoteIssueSnapshot) contains(agentIssueID string) bool {
	if s == nil {
		return false
	}
	_, ok := s.issueIDs[remoteIssueID(s.orgID, agentIssueID)]
	return ok
}

func remoteIssueID(orgID int64, agentIssueID string) string {
	return murmurUUID(fmt.Sprintf("%d:%s", orgID, agentIssueID))
}

func murmurUUID(value string) string {
	h1, h2 := murmur3.StringSum128(value)
	hash := fmt.Sprintf("%016x%016x", h1, h2)
	return fmt.Sprintf("%s-%s-%s-%s-%s", hash[:8], hash[8:12], hash[12:16], hash[16:20], hash[20:])
}
