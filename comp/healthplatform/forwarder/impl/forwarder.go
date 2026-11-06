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
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	intakeEndpointPrefix = "https://agenthealth-intake."
	intakeEndpointPath   = "/api/v2/agenthealth"
	intakeURLOverrideKey = "health_platform.forwarder.dd_url"
	httpTimeout          = 30 * time.Second

	// forwarderName tags this forwarder in the shared forwarder.http_errors metric.
	forwarderName = "health_platform"
)

// forwarder is a stateless HTTP client. It has no ticker or provider — the
// egress component owns the send cadence.
type forwarder struct {
	cfg        pkgconfigmodel.Reader
	intakeURL  string
	httpClient *http.Client
	log        log.Component
	httpErrors telemetry.Counter
}

// Requires defines the dependencies for the forwarder.
type Requires struct {
	Log       log.Component
	Config    config.Component
	Telemetry telemetry.Component
}

// newHTTPErrorsCounter registers the forwarder.http_errors counter. Its name and tags are meant to
// be shared by every forwarder, so that agent telemetry can report HTTP errors across forwarders.
func newHTTPErrorsCounter(tel telemetry.Component) telemetry.Counter {
	return tel.NewCounter(
		"forwarder",
		"http_errors",
		[]string{"forwarder", "code"},
		"Number of HTTP error responses received from a Datadog intake, by forwarder and status class",
	)
}

// NewComponent creates a forwarder. It never registers lifecycle hooks; there is nothing
// to clean up for a stateless HTTP client.
func NewComponent(reqs Requires) forwarderdef.Component {
	return &forwarder{
		cfg:        reqs.Config,
		intakeURL:  buildIntakeURL(reqs.Config),
		httpClient: buildHTTPClient(reqs.Config),
		log:        reqs.Log,
		httpErrors: newHTTPErrorsCounter(reqs.Telemetry),
	}
}

// Send marshals report and POSTs it to the Datadog intake. It returns the
// number of payload bytes sent on success, or 0 alongside a non-nil error.
func (f *forwarder) Send(ctx context.Context, report *healthplatform.HealthReport) (int, error) {
	apiKey := f.cfg.GetString("api_key")
	if apiKey == "" {
		return 0, errors.New("API key not configured")
	}

	payload, err := json.Marshal(report)
	if err != nil {
		return 0, fmt.Errorf("marshal report: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, f.intakeURL, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("DD-Agent-Version", version.AgentVersion)
	req.Header.Set("User-Agent", "datadog-agent/"+version.AgentVersion)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if class := statusClass(resp.StatusCode); class != "" {
			f.httpErrors.Inc(forwarderName, class)
		}
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return len(payload), nil
}

// statusClass returns the value of the `code` tag of forwarder.http_errors for an HTTP status code
// ("4xx" or "5xx"), or "" when the status is not an HTTP error and must not be counted.
func statusClass(statusCode int) string {
	if statusCode >= 400 && statusCode < 500 {
		return "4xx"
	}
	if statusCode >= 500 && statusCode < 600 {
		return "5xx"
	}
	return ""
}

// buildIntakeURL returns the URL health reports are POSTed to: the health platform intake of `site`,
// unless overridden by `health_platform.forwarder.dd_url`. Like the other dedicated intakes (APM,
// logs, remote config), it does not follow the main `dd_url`, which only overrides the metrics intake.
func buildIntakeURL(cfg pkgconfigmodel.Reader) string {
	return configutils.GetMainEndpoint(cfg, intakeEndpointPrefix, intakeURLOverrideKey) + intakeEndpointPath
}

func buildHTTPClient(cfg pkgconfigmodel.Reader) *http.Client {
	return &http.Client{
		Timeout:   httpTimeout,
		Transport: httputils.CreateHTTPTransport(cfg),
	}
}
