// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package invalidconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/dustin/go-humanize/english"
	"github.com/qri-io/jsonpointer"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const (
	// FallbackIssueName identifies confirmed replacement of invalid configuration.
	FallbackIssueName = "Config Fallback"
	// FallbackIssueType is the backend type for core-Agent fallback reports.
	FallbackIssueType = "config_fallback"
	// FallbackIssueID is scoped by host and reporting process.
	FallbackIssueID = "config-fallback"
	// SystemProbeFallbackIssueName separates system-probe ownership from core startup resolution.
	SystemProbeFallbackIssueName = "System Probe Config Fallback"
	// SystemProbeFallbackIssueType identifies fallback decisions in system-probe.
	SystemProbeFallbackIssueType = "system_probe_config_fallback"
	contextKeyFallbacks          = "fallbacks"
)

type fallbackModule struct{ deps issues.ModuleDeps }

func newFallbackModule(deps issues.ModuleDeps) issues.Module                     { return &fallbackModule{deps: deps} }
func (*fallbackModule) IssueName() string                                        { return FallbackIssueName }
func (*fallbackModule) IssueType() string                                        { return FallbackIssueType }
func (*fallbackModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck { return nil }
func (*fallbackModule) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	var fallbacks []model.ConfigFallback
	if err := json.Unmarshal([]byte(ctx[contextKeyFallbacks]), &fallbacks); err != nil {
		return nil, err
	}
	return BuildFallbackIssue("agent", ctx[contextKeyConfigPath], fallbacks)
}
func (m *fallbackModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{Interval: time.Minute, BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
		Source: FallbackIssueID,
		Fn: func() ([]runnerdef.IssueReport, error) {
			if !m.deps.Config.GetBool("health_platform.invalidconfig_check.enabled") {
				return nil, nil
			}
			fallbacks := m.deps.Config.GetConfigFallbacks()
			if len(fallbacks) == 0 {
				return nil, nil
			}
			encoded, err := json.Marshal(fallbacks)
			if err != nil {
				return nil, err
			}
			return []runnerdef.IssueReport{{Source: "agent", IssueName: FallbackIssueName,
				IssueID: ConfigAdjustmentIssueID(FallbackIssueID, "agent", m.deps.Hostname.GetSafe(context.Background())),
				Context: map[string]string{contextKeyFallbacks: string(encoded), contextKeyConfigPath: m.deps.Config.ConfigFileUsed()},
			}}, nil
		},
	}}
}

// FilterConfigConversions keeps a separate conversion notice from hiding a final fallback for the same setting.
func FilterConfigConversions(conversions []model.ConfigTypeConversion, fallbacks []model.ConfigFallback) []model.ConfigTypeConversion {
	keys := make(map[string]bool, len(fallbacks))
	for _, fallback := range fallbacks {
		keys[fallback.Key] = true
	}
	return slices.DeleteFunc(conversions, func(conversion model.ConfigTypeConversion) bool { return keys[conversion.Key] })
}

// BuildFallbackIssue reports code-defined defaults or the setting a derived replacement depends on, never that setting's value.
func BuildFallbackIssue(component, configPath string, fallbacks []model.ConfigFallback) (*healthplatform.Issue, error) {
	if len(fallbacks) == 0 {
		return nil, errors.New("no configuration fallbacks")
	}
	name, issueName, issueType := "The Agent", FallbackIssueName, FallbackIssueType
	if component == "system-probe" {
		name, issueName, issueType = "System-probe", SystemProbeFallbackIssueName, SystemProbeFallbackIssueType
	}
	descriptions, fixes, details := []string{}, []string{}, []any{}
	seen := map[string]bool{}
	for _, fallback := range fallbacks {
		path := scrubViolationPath(jsonpointer.Pointer(strings.Split(fallback.Key, ".")).String())
		detail := map[string]any{"path": path, "reason": fallback.Reason, "source": fallback.Source.String()}
		description := fmt.Sprintf("%s %s. %s is using ", inlineCode(path), fallback.Reason, name)
		fix := fmt.Sprintf("Correct %s: the setting %s.", inlineCode(path), fallback.Reason)
		if fallback.DefaultValue != nil {
			value, err := json.Marshal(fallback.DefaultValue)
			if err != nil {
				return nil, err
			}
			description += "the default value of " + inlineCode(string(value)) + "."
			fix += " You can also remove the setting to use its default."
			detail["default_value"] = fallback.DefaultValue
		} else {
			replacement := scrubViolationPath(jsonpointer.Pointer(strings.Split(fallback.ReplacementSetting, ".")).String())
			description += "a replacement based on " + inlineCode(replacement) + "."
			detail["replacement_setting"] = replacement
		}
		// Several active transports can make the same decision; customers only need one explanation.
		if seen[description] {
			continue
		}
		seen[description] = true
		descriptions, fixes, details = append(descriptions, description), append(fixes, fix), append(details, detail)
	}
	configPath, _ = scrubber.ScrubString(configPath)
	where := "Check the settings listed below in your Agent configuration file or environment variables."
	if configPath != "" {
		where = "Check the settings listed below in " + inlineCode(configPath) + " or in environment variables."
	}
	extra, err := structpb.NewStruct(map[string]any{contextKeyConfigPath: configPath, contextKeyFallbacks: details})
	if err != nil {
		return nil, err
	}
	return &healthplatform.Issue{IssueName: issueName, IssueType: issueType,
		Title:       fmt.Sprintf("%s replaced %d invalid configuration %s", name, len(details), english.PluralWord(len(details), "setting", "settings")),
		Description: strings.Join(descriptions, " "), Category: "configuration", Location: component, Source: component,
		Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, Tags: []string{"config", "fallback"}, Extra: extra,
		Remediation: &healthplatform.Remediation{Summary: "Correct the settings or remove them, then restart " + strings.ToLower(name) + ".",
			Steps: []*healthplatform.RemediationStep{{Order: 1, Text: where}, {Order: 2, Text: strings.Join(fixes, "\n")}, {Order: 3, Text: "Restart " + strings.ToLower(name) + "."}}},
	}, nil
}
