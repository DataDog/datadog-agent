// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteimpl implements the remote health-platform issue loader.
package remoteimpl

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v7"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	remotedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/remote/def"
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
	remoteIssuesResourceType     = "agent_health_issues"
	remoteIssuesHTTPTimeout      = 10 * time.Second
	remoteIssuesIdentityTimeout  = 10 * time.Second
	remoteIssuesMaxResponse      = 10 * 1024 * 1024
	jsonAPIContentType           = "application/vnd.api+json"
)

type remoteIssueLoader struct {
	config      config.Component
	log         log.Component
	agentFlavor string
	agentType   string
	resourceID  func(context.Context) string
	baseURL     string
	httpClient  *http.Client
}

var _ remotedef.Component = (*remoteIssueLoader)(nil)

type remoteIssuesResponse struct {
	Data *remoteIssueResource `json:"data"`
}

type remoteIssueResource struct {
	ID         string                `json:"id"`
	Type       string                `json:"type"`
	Attributes remoteIssueAttributes `json:"attributes"`
}

type remoteIssueAttributes struct {
	Issues *[]remoteIssue `json:"issues"`
}

type remoteIssue struct {
	IssueID   string `json:"issue_id"`
	IssueName string `json:"issue_name"`
	IssueType string `json:"issue_type"`
	Category  string `json:"category"`
}

type remoteResourceIdentity interface {
	DeploymentIDWithContext(context.Context) string
	ClusterIDWithContext(context.Context) string
}

// Requires defines the dependencies for the remote issue loader component.
type Requires struct {
	Config       config.Component
	Log          log.Component
	Workloadmeta workloadmeta.Component `optional:"true"`
}

// Provides defines the output of the remote issue loader component.
type Provides struct {
	Comp remotedef.Component
}

// NewComponent creates the remote issue loader component.
func NewComponent(reqs Requires) Provides {
	return Provides{
		Comp: newRemoteIssueLoader(reqs, flavor.GetFlavor(), selfident.New(reqs.Workloadmeta)),
	}
}

func newRemoteIssueLoader(reqs Requires, agentFlavor string, identity remoteResourceIdentity) *remoteIssueLoader {
	site := strings.TrimSpace(reqs.Config.GetString("site"))
	loader := &remoteIssueLoader{
		config:      reqs.Config,
		log:         reqs.Log,
		agentFlavor: agentFlavor,
		baseURL:     configutils.BuildURLWithPrefix(remoteIssuesEndpointPrefix, site),
		httpClient: &http.Client{
			Timeout:       remoteIssuesHTTPTimeout,
			Transport:     httputils.CreateHTTPTransport(reqs.Config),
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	if agentFlavor == flavor.ClusterAgent {
		loader.agentType = remoteIssuesClusterAgentType
		loader.resourceID = identity.ClusterIDWithContext
	} else {
		loader.agentType = remoteIssuesNodeAgentType
		loader.resourceID = identity.DeploymentIDWithContext
	}
	return loader
}

// Load retrieves active issue lifecycle metadata from the Agent Health API.
func (r *remoteIssueLoader) Load(ctx context.Context) ([]remotedef.Issue, error) {
	if confighelper.IsCLCRunner(r.config) {
		r.log.Info("Running on Kubernetes: remote health platform restoration disabled for this process")
		return nil, nil
	}
	if r.agentFlavor != flavor.DefaultAgent && r.agentFlavor != flavor.ClusterAgent {
		r.log.Info("Running on Kubernetes: remote health platform restoration disabled for this process")
		return nil, nil
	}
	if r.config.GetBool("fips.enabled") {
		r.log.Info("Running on Kubernetes: remote health platform restoration is unsupported in FIPS mode")
		return nil, nil
	}
	if r.config.GetBool("skip_ssl_validation") {
		r.log.Info("Running on Kubernetes: remote health platform restoration requires TLS certificate verification")
		return nil, nil
	}

	apiKey := configutils.SanitizeAPIKey(r.config.GetString("api_key"))
	if apiKey == "" {
		r.log.Info("Running on Kubernetes: remote health platform restoration requires api_key")
		return nil, nil
	}

	identityCtx, cancel := context.WithTimeout(ctx, remoteIssuesIdentityTimeout)
	resourceID := strings.TrimSpace(r.resourceID(identityCtx))
	identityErr := identityCtx.Err()
	cancel()
	if identityErr != nil {
		return nil, fmt.Errorf("resolve remote issue resource ID: %w", identityErr)
	}
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

	// Share the existing HTTP timeout across all attempts, including backoff
	// and response reads. Restore before checks start so reconciliation sees
	// the snapshot; do not merge a late snapshot into live issue state.
	retryCtx, cancel := context.WithTimeout(ctx, remoteIssuesHTTPTimeout)
	defer cancel()
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 500 * time.Millisecond
	bo.MaxInterval = 2 * time.Second
	return backoff.Retry(retryCtx, func() ([]remotedef.Issue, error) {
		return r.fetchOnce(req.Clone(retryCtx), resourceID)
	}, backoff.WithBackOff(bo), backoff.WithMaxElapsedTime(0))
}

func (r *remoteIssueLoader) fetchOnce(req *http.Request, resourceID string) ([]remotedef.Issue, error) {
	if err := req.Context().Err(); err != nil {
		return nil, backoff.Permanent(err)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		var certErr *tls.CertificateVerificationError
		if errors.As(err, &certErr) {
			return nil, backoff.Permanent(fmt.Errorf("load remote issues: %w", err))
		}
		return nil, fmt.Errorf("load remote issues: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("remote issue request returned status %d", resp.StatusCode)
		if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			// Honor a server-requested delay; the request context still bounds
			// the entire operation even when Retry-After exceeds the budget.
			if value := resp.Header.Get("Retry-After"); value != "" {
				if delay, parseErr := time.ParseDuration(value + "s"); parseErr == nil && delay >= 0 {
					return nil, backoff.RetryAfter(delay, err)
				}
				if deadline, parseErr := http.ParseTime(value); parseErr == nil {
					return nil, backoff.RetryAfter(time.Until(deadline), err)
				}
			}
			return nil, err
		}
		return nil, backoff.Permanent(err)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, remoteIssuesMaxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read remote issue response: %w", err)
	}
	if len(body) > remoteIssuesMaxResponse {
		return nil, backoff.Permanent(fmt.Errorf("remote issue response exceeds %d bytes", remoteIssuesMaxResponse))
	}

	var response remoteIssuesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, backoff.Permanent(fmt.Errorf("decode remote issue response: %w", err))
	}
	if response.Data == nil {
		return nil, backoff.Permanent(errors.New("remote issue response must contain data"))
	}
	resource := response.Data
	if resource.Type != remoteIssuesResourceType {
		return nil, backoff.Permanent(fmt.Errorf("remote issue response has unexpected type %q", resource.Type))
	}
	if resource.ID != resourceID {
		return nil, backoff.Permanent(fmt.Errorf("remote issue response has resource ID %q, expected %q", resource.ID, resourceID))
	}
	if resource.Attributes.Issues == nil {
		return nil, backoff.Permanent(errors.New("remote issue response has no issues"))
	}

	issues := make([]remotedef.Issue, 0, len(*resource.Attributes.Issues))
	for _, issue := range *resource.Attributes.Issues {
		issues = append(issues, remotedef.Issue{
			IssueID:   issue.IssueID,
			IssueName: issue.IssueName,
			IssueType: issue.IssueType,
			Category:  issue.Category,
		})
	}
	return issues, nil
}
