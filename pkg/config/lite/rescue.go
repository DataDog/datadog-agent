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
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/pkg/config/schema"
)

const (
	// rescueHTTPTimeout bounds the entire candidate-iteration loop, well under
	// typical service-manager start timeouts (systemd default is 90s).
	rescueHTTPTimeout  = 10 * time.Second
	rescueIntakePrefix = "https://agenthealth-intake."
	rescueIntakePath   = "/api/v2/agenthealth"
	rescueEventType    = "agent-health-issues"

	// rescueFlavor tags rescue payloads with a static service name: the real
	// flavor isn't always knowable at rescue time (Fx hasn't initialised), and
	// a stable placeholder keeps backend dedup stable.
	rescueFlavor = "agent"

	// rescueMaxAttempts caps the api_key candidates posted to the intake.
	// Defends against a crafted datadog.yaml using the intake as a credential
	// validation oracle.
	rescueMaxAttempts = 3
)

// validSite restricts intake site values to DNS-label syntax. Rejects schemes,
// path separators, userinfo, fragments — anything that could redirect the
// rescue POST (with DD-API-KEY attached) outside agenthealth-intake.*.
var validSite = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*\.[a-z]{2,}$`)

// rescueHTTPClient refuses redirects. Go's stdlib strips Authorization on
// cross-host 3xx but NOT custom headers, so otherwise DD-API-KEY would leak
// to the redirect target.
var rescueHTTPClient = &http.Client{
	Timeout: rescueHTTPTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// Rescue is the one-shot orchestrator invoked from the agent's failure path
// (defer/recover or returned Fx error). It runs the resolver pipeline,
// schema-validates any config it could parse, and best-effort POSTs an Agent
// Health issue to the Datadog intake.
//
// Rescue never panics. On any internal failure it returns the wrapped error
// for the caller to log; it must not gate process exit on the result.
func Rescue(ctx context.Context, cliConfPath, defaultConfPath string, startupErr error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("rescue: recovered panic: %v", r)
		}
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	rctx, cancel := context.WithTimeout(ctx, rescueHTTPTimeout)
	defer cancel()

	cfg := Extract(rctx, cliConfPath, defaultConfPath)
	return rescueWithURL(rctx, cfg, IntakeURL(cfg.Site.Value), startupErr)
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

// IntakeURL builds the rescue POST target for site. Falls back to DefaultSite
// when site is empty or fails DNS-label validation, preventing an attacker-
// controlled `site:` value from redirecting the request to an arbitrary host.
func IntakeURL(site string) string {
	site = strings.ToLower(strings.TrimRight(site, "/"))
	if !validSite.MatchString(site) {
		site = DefaultSite
	}
	return rescueIntakePrefix + site + rescueIntakePath
}

// rescueWithURL is the URL-decoupled core of Rescue. Tests target it directly
// to route POSTs at an httptest.Server without stubbing module-level state.
//
// Walks the api_key candidates best-to-worst (primary + any fuzzy alternates)
// and stops at the first 2xx, capped at rescueMaxAttempts. The intake decides
// which credential is right — fuzzy collisions like `app_key` vs `api_kye`
// self-heal here instead of being blocked by a static denylist.
func rescueWithURL(ctx context.Context, cfg LiteConfig, url string, startupErr error) error {
	issue := buildRescueIssue(cfg, startupErr)
	if issue == nil {
		return nil
	}

	issue.DetectedAt = time.Now().UTC().Format(time.RFC3339)
	hostname, _ := os.Hostname()
	report := &healthplatform.HealthReport{
		EventType: rescueEventType,
		EmittedAt: issue.DetectedAt,
		Service:   rescueFlavor,
		Host:      &healthplatform.HostInfo{Hostname: hostname},
		Issues:    map[string]*healthplatform.Issue{IssueID: issue},
	}

	var lastErr error
	attempts := 0
	for _, c := range append([]ConfigField{cfg.APIKey}, cfg.APIKeyCandidates...) {
		if !c.resolved() {
			continue
		}
		if attempts >= rescueMaxAttempts {
			break
		}
		attempts++
		err := postRescue(ctx, url, c.Value, report)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	if attempts == 0 {
		return fmt.Errorf("rescue: no usable api_key resolved (source=%s)", cfg.APIKey.Source)
	}
	return lastErr
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
			return BuildInvalidConfigIssue(IssueInfo{
				Kind:       ErrorKindSchemaValidation,
				ConfigPath: cfg.ConfigFilePath,
				Errors:     strings.Join(errs, "\n"),
				ErrorCount: len(errs),
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

	resp, err := rescueHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("send rescue request: %w", err)
	}
	// Drain so the connection can be reused across candidate retries.
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("rescue intake returned status %d", resp.StatusCode)
	}
	return nil
}
