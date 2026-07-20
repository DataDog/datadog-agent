// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package forwarderimpl implements the health platform forwarder component.
package forwarderimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	intakeEndpointPrefix = "https://agenthealth-intake."
	intakeEndpointPath   = "/api/v2/agenthealth"
	httpTimeout          = 30 * time.Second
)

// forwarder is a stateless HTTP client. It has no ticker or provider — the
// egress component owns the send cadence.
type forwarder struct {
	cfg        pkgconfigmodel.Reader
	intakeURL  string
	httpClient *http.Client
	log        log.Component
}

// Requires defines the dependencies for the forwarder.
type Requires struct {
	Log    log.Component
	Config config.Component
}

// NewComponent creates a forwarder. It never registers lifecycle hooks; there is nothing
// to clean up for a stateless HTTP client.
func NewComponent(reqs Requires) forwarderdef.Component {
	return &forwarder{
		cfg:        reqs.Config,
		intakeURL:  buildIntakeURL(reqs.Config),
		httpClient: buildHTTPClient(reqs.Config),
		log:        reqs.Log,
	}
}

// Send marshals report and POSTs it to the Datadog intake.
func (f *forwarder) Send(ctx context.Context, report *healthplatform.HealthReport) error {
	apiKey := f.cfg.GetString("api_key")
	if apiKey == "" {
		return errors.New("API key not configured")
	}

	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, f.intakeURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("DD-Agent-Version", version.AgentVersion)
	req.Header.Set("User-Agent", "datadog-agent/"+version.AgentVersion)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func buildIntakeURL(cfg pkgconfigmodel.Reader) string {
	baseURL := configutils.GetMainEndpoint(cfg, intakeEndpointPrefix, "dd_url")
	return baseURL + intakeEndpointPath
}

func buildHTTPClient(cfg pkgconfigmodel.Reader) *http.Client {
	return &http.Client{
		Timeout:   httpTimeout,
		Transport: httputils.CreateHTTPTransport(cfg),
	}
}
