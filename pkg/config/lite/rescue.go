// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/pkg/config/schema"
)

const (
	rescueHTTPTimeout  = 3 * time.Second
	rescueIntakePrefix = "https://agenthealth-intake."
	rescueIntakePath   = "/api/v2/agenthealth"
	rescueEventType    = "agent-health-issues"

	// rescueFlavor tags rescue payloads with a static service name: the real
	// flavor isn't always knowable at rescue time (Fx hasn't initialised), and
	// a stable placeholder keeps backend dedup stable.
	rescueFlavor = "agent"
)

// Rescue is the one-shot orchestrator invoked from the agent's failure path
// (defer/recover or returned Fx error). It runs the resolver pipeline,
// schema-validates any config it could parse, and best-effort POSTs an Agent
// Health issue to the Datadog intake.
//
// Rescue never panics. On any internal failure it returns the wrapped error
// for the caller to log; it must not gate process exit on the result.
func Rescue(ctx context.Context, cliConfPath, defaultConfPath string, startupErr error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	rctx, cancel := context.WithTimeout(ctx, rescueHTTPTimeout)
	defer cancel()

	cfg := Extract(rctx, cliConfPath, defaultConfPath)
	return rescueWithURL(rctx, cfg, intakeURL(cfg.Site.Value), startupErr)
}

// DefaultConfigPath returns the platform-default location of datadog.yaml.
// Callers can pass it as defaultConfPath when invoking Rescue.
func DefaultConfigPath() string {
	if p := os.Getenv("DD_CONFIG"); p != "" {
		return p
	}
	return defaultConfigPathForGOOS(runtime.GOOS)
}

func defaultConfigPathForGOOS(goos string) string {
	switch goos {
	case "darwin":
		return "/opt/datadog-agent/etc"
	case "windows":
		return `C:\ProgramData\Datadog`
	default:
		return "/etc/datadog-agent"
	}
}

func intakeURL(site string) string {
	if site == "" {
		site = DefaultSite
	}
	return rescueIntakePrefix + strings.TrimRight(site, "/") + rescueIntakePath
}

// rescueWithURL is the URL-decoupled core of Rescue. Tests target it directly
// to route POSTs at an httptest.Server without stubbing module-level state.
func rescueWithURL(ctx context.Context, cfg LiteConfig, url string, startupErr error) error {
	issue := buildRescueIssue(cfg, startupErr)
	if issue == nil {
		return nil
	}
	if !cfg.APIKey.resolved() {
		return fmt.Errorf("rescue: no usable api_key resolved (source=%s)", cfg.APIKey.Source)
	}

	issue.DetectedAt = time.Now().UTC().Format(time.RFC3339)
	hostname, _ := os.Hostname()
	report := &healthplatform.HealthReport{
		EventType: rescueEventType,
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Service:   rescueFlavor,
		Host:      &healthplatform.HostInfo{Hostname: hostname},
		Issues:    map[string]*healthplatform.Issue{"datadog.yaml": issue},
	}
	return postRescue(ctx, url, cfg.APIKey.Value, report)
}

// buildRescueIssue inspects cfg + startupErr and produces the Issue payload.
// Returns nil if there is nothing worth reporting. Priority: yaml_parse >
// schema_validation > startup_failure.
func buildRescueIssue(cfg LiteConfig, startupErr error) *healthplatform.Issue {
	if cfg.YAMLParseErr != nil {
		return BuildInvalidConfigIssue(IssueInfo{
			Kind:         ErrorKindYAMLParse,
			ConfigPath:   cfg.ConfigFilePath,
			ErrorMessage: cfg.YAMLParseErr.Error(),
		})
	}
	if cfg.ParsedConfig != nil {
		if errs, _ := schema.ValidateCoreConfig(cfg.ParsedConfig); len(errs) > 0 {
			visible, truncated := TruncateSchemaErrors(errs)
			return BuildInvalidConfigIssue(IssueInfo{
				Kind:       ErrorKindSchemaValidation,
				ConfigPath: cfg.ConfigFilePath,
				Errors:     strings.Join(visible, "\n"),
				ErrorCount: len(errs),
				Truncated:  truncated,
			})
		}
	}
	if startupErr != nil {
		return BuildInvalidConfigIssue(IssueInfo{
			Kind:         ErrorKindStartupFailure,
			ConfigPath:   cfg.ConfigFilePath,
			ErrorMessage: startupErr.Error(),
		})
	}
	return nil
}

func postRescue(ctx context.Context, url, apiKey string, report *healthplatform.HealthReport) error {
	if apiKey == "" {
		return errors.New("rescue: api_key is empty")
	}

	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal rescue payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build rescue request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("User-Agent", "datadog-agent-lite")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send rescue request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("rescue intake returned status %d", resp.StatusCode)
	}
	return nil
}
