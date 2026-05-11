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

	// rescueFlavor is the service name we tag rescue payloads with. The real
	// flavor isn't always knowable at rescue time (Fx hasn't initialised), so
	// a static placeholder keeps backend dedup stable.
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

	issue := buildRescueIssue(cfg, startupErr)
	if issue == nil {
		return nil
	}

	if !cfg.APIKey.resolved() {
		return fmt.Errorf("rescue: no usable api_key resolved (source=%s)", cfg.APIKey.Source)
	}

	report := &healthplatform.HealthReport{
		EventType: rescueEventType,
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Service:   rescueFlavor,
		Host:      &healthplatform.HostInfo{Hostname: hostnameOrEmpty()},
		Issues:    map[string]*healthplatform.Issue{"datadog.yaml": issue},
	}

	return postRescue(rctx, cfg.Site.Value, cfg.APIKey.Value, report)
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

// buildRescueIssue inspects cfg + startupErr and produces the appropriate
// Issue payload. Returns nil if there is nothing worth reporting.
func buildRescueIssue(cfg LiteConfig, startupErr error) *healthplatform.Issue {
	switch {
	case cfg.YAMLParseErr != nil:
		issue := BuildInvalidConfigIssue(IssueInfo{
			Kind:         ErrorKindYAMLParse,
			ConfigPath:   cfg.ConfigFilePath,
			ErrorMessage: cfg.YAMLParseErr.Error(),
		})
		stampDetectedAt(issue)
		return issue

	case cfg.ParsedConfig != nil:
		errs, _ := schema.ValidateCoreConfig(cfg.ParsedConfig)
		if len(errs) > 0 {
			return rescueSchemaIssue(cfg.ConfigFilePath, errs)
		}
		if startupErr != nil {
			return rescueStartupIssue(cfg.ConfigFilePath, startupErr)
		}
		return nil

	case startupErr != nil:
		return rescueStartupIssue(cfg.ConfigFilePath, startupErr)

	default:
		return nil
	}
}

func rescueSchemaIssue(path string, errs []string) *healthplatform.Issue {
	visible, truncated := TruncateSchemaErrors(errs)
	issue := BuildInvalidConfigIssue(IssueInfo{
		Kind:       ErrorKindSchemaValidation,
		ConfigPath: path,
		Errors:     strings.Join(visible, "\n"),
		ErrorCount: len(errs),
		Truncated:  truncated,
	})
	stampDetectedAt(issue)
	return issue
}

func rescueStartupIssue(path string, startupErr error) *healthplatform.Issue {
	issue := BuildInvalidConfigIssue(IssueInfo{
		Kind:         ErrorKindStartupFailure,
		ConfigPath:   path,
		ErrorMessage: startupErr.Error(),
	})
	stampDetectedAt(issue)
	return issue
}

// stampDetectedAt sets the rescue timestamp. The in-Fx path leaves DetectedAt
// empty and the store fills it; rescue bypasses the store so it stamps here.
func stampDetectedAt(issue *healthplatform.Issue) {
	if issue != nil {
		issue.DetectedAt = time.Now().UTC().Format(time.RFC3339)
	}
}

// buildIntakeURL is a var so tests can stub the destination.
var buildIntakeURL = func(site string) string {
	if site == "" {
		site = DefaultSite
	}
	return rescueIntakePrefix + strings.TrimRight(site, "/") + rescueIntakePath
}

func postRescue(ctx context.Context, site, apiKey string, report *healthplatform.HealthReport) error {
	if apiKey == "" {
		return errors.New("rescue: api_key is empty")
	}

	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal rescue payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildIntakeURL(site), bytes.NewReader(payload))
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

func hostnameOrEmpty() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}
