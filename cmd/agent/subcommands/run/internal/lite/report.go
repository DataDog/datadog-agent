// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package lite

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/client"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueName     = "Agent Startup Failure"
	issueType     = "agent_startup_failure"
	rescueTimeout = 10 * time.Second
)

// Rescue makes one best-effort report for a failed startup.
// Its caller must preserve startupErr regardless of the reporting result.
func Rescue(ctx context.Context, params Params, startupErr error) error {
	if startupErr == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, rescueTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	cfg, path, err := recoverConfig(params)
	if err != nil {
		return err
	}
	if err = validateSettings(cfg); err != nil {
		return err
	}
	setup.LoadProxyFromEnv(cfg)
	for _, key := range reportingKeys {
		cfg.rememberSensitive(key, cfg.Get(key))
	}
	if err = resolveSecrets(ctx, cfg); err != nil {
		return err
	}
	cfg.sanitizeAPIKey()
	if err = mergeFleetConfig(cfg, params); err != nil {
		return err
	}
	if err = validateSettings(cfg); err != nil {
		return err
	}
	if !cfg.GetBool("health_platform.enabled") {
		return nil
	}
	if err = validateDestination(cfg); err != nil {
		return err
	}
	cfg.sanitizeAPIKey()
	key := cfg.GetString("api_key")
	if key == "" || strings.Contains(key, "ENC[") {
		return errors.New("no usable reporting API key")
	}
	hostname := cfg.GetString("hostname")
	if hostname == "" {
		hostname, err = os.Hostname()
	}
	if err != nil || hostname == "" {
		return errors.New("no reporting hostname")
	}
	report := startupReport(hostname, path, scrubError(startupErr, cfg))
	_, err = client.New(cfg).Send(ctx, report)
	if err != nil {
		return errors.New("startup health report could not be delivered")
	}
	return nil
}

func scrubError(err error, cfg *reportingConfig) string {
	message := cfg.redact(err.Error())
	clean, scrubErr := scrubber.ScrubString(message)
	if scrubErr != nil {
		return "Agent startup failed; consult the local Agent logs."
	}
	return clean
}

func startupReport(hostname, path, message string) *healthplatform.HealthReport {
	identity := sha256.Sum256([]byte(hostname + "\x00" + path))
	id := fmt.Sprintf("agent-startup-failure:%x", identity[:16])
	now := time.Now().UTC().Format(time.RFC3339)
	extra, _ := structpb.NewStruct(map[string]interface{}{"config_path": path, "error_message": message})
	issue := &healthplatform.Issue{
		Id: id, IssueName: issueName, IssueType: issueType,
		Title:       "Datadog Agent failed to start on " + hostname,
		Description: "The Agent could not complete startup: " + message,
		Category:    "startup", Location: "agent", Source: "agent",
		Severity:   healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		DetectedAt: now, Extra: extra, Tags: []string{"agent", "startup_failure"},
		Remediation: &healthplatform.Remediation{
			Summary: "Correct the startup error in the local Agent logs, then restart the Agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Run `datadog-agent status` and inspect the local Agent startup logs."},
				{Order: 2, Text: "Correct the reported configuration, permissions, or resource conflict."},
				{Order: 3, Text: "Restart the Datadog Agent and run `datadog-agent status` to verify startup."},
			},
		},
	}
	return &healthplatform.HealthReport{
		EventType: "agent-health-issues", EmittedAt: now, Service: "agent",
		Host: &healthplatform.HostInfo{Hostname: hostname, AgentVersion: &version.AgentVersion}, Issues: map[string]*healthplatform.Issue{id: issue},
	}
}
