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
	"hash/fnv"
	"strings"

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
	// ConversionIssueName identifies loader type corrections, separately from schema errors.
	ConversionIssueName = "Config Type Conversion"
	// ConversionIssueType is the backend type for conversion reports.
	ConversionIssueType = "config_type_conversion"
	// ConversionIssueID is scoped by process and host before reporting.
	ConversionIssueID = "config-type-conversion"
	// SystemProbeConversionIssueName keeps core startup resolution from owning system-probe reports.
	SystemProbeConversionIssueName = "System Probe Config Type Conversion"
	// SystemProbeConversionIssueType identifies conversions observed by system-probe itself.
	SystemProbeConversionIssueType = "system_probe_config_type_conversion"
	contextKeyConversions          = "conversions"
)

type conversionModule struct{ deps issues.ModuleDeps }

func newConversionModule(deps issues.ModuleDeps) issues.Module                     { return &conversionModule{deps: deps} }
func (*conversionModule) IssueName() string                                        { return ConversionIssueName }
func (*conversionModule) IssueType() string                                        { return ConversionIssueType }
func (*conversionModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck { return nil }

func (*conversionModule) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	var conversions []model.ConfigTypeConversion
	if err := json.Unmarshal([]byte(ctx[contextKeyConversions]), &conversions); err != nil {
		return nil, err
	}
	return BuildConversionIssue("agent", ctx[contextKeyConfigPath], conversions)
}

func (m *conversionModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
		Source: ConversionIssueID,
		Fn: func() ([]runnerdef.IssueReport, error) {
			if !m.deps.Config.GetBool("health_platform.invalidconfig_check.enabled") {
				return nil, nil
			}
			conversions := m.deps.Config.GetConfigTypeConversions()
			if len(conversions) == 0 {
				return nil, nil
			}
			encoded, err := json.Marshal(conversions)
			if err != nil {
				return nil, err
			}
			return []runnerdef.IssueReport{{
				Source:    "agent",
				IssueID:   ConfigAdjustmentIssueID(ConversionIssueID, "agent", m.deps.Hostname.GetSafe(context.Background())),
				IssueName: ConversionIssueName,
				Context:   map[string]string{contextKeyConversions: string(encoded), contextKeyConfigPath: m.deps.Config.ConfigFileUsed()},
			}}, nil
		},
	}}
}

// ConfigAdjustmentIssueID stays stable when the config filename or individual settings change.
func ConfigAdjustmentIssueID(base, component, hostname string) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%s", component, hostname)
	return fmt.Sprintf("%s:%016x", base, h.Sum64())
}

// BuildConversionIssue explains successful conversions without including the input or converted values.
func BuildConversionIssue(component, configPath string, conversions []model.ConfigTypeConversion) (*healthplatform.Issue, error) {
	if len(conversions) == 0 {
		return nil, errors.New("no configuration type conversions")
	}
	name := "The Agent"
	issueName, issueType := ConversionIssueName, ConversionIssueType
	if component == "system-probe" {
		name = "System-probe"
		issueName, issueType = SystemProbeConversionIssueName, SystemProbeConversionIssueType
	}
	descriptions, fixes := make([]string, 0, len(conversions)), make([]string, 0, len(conversions))
	details := make([]any, 0, len(conversions))
	for _, conversion := range conversions {
		from, to := typeLabels[conversion.FromType], typeLabels[conversion.ToType]
		if from == "" || to == "" {
			return nil, errors.New("unsupported configuration conversion type")
		}
		path := scrubViolationPath(jsonpointer.Pointer(strings.Split(conversion.Key, ".")).String() + conversion.Path)
		descriptions = append(descriptions, fmt.Sprintf("%s was provided as %s. %s converted it to %s while loading the configuration.", inlineCode(path), from, name, to))
		fix := fmt.Sprintf("Write %s as %s.", inlineCode(path), to)
		if conversion.ToType == "integer" || conversion.ToType == "boolean" {
			fix = fmt.Sprintf("Write %s as %s without quotation marks.", inlineCode(path), to)
		}
		if conversion.ToType == "string" {
			fix = fmt.Sprintf("Enclose %s in quotation marks so it is a string.", inlineCode(path))
		}
		fixes = append(fixes, fix)
		details = append(details, map[string]any{"path": path, "source": conversion.Source.String(), "from_type": conversion.FromType, "to_type": conversion.ToType})
	}
	configPath, _ = scrubber.ScrubString(configPath)
	where := "Check the settings listed below in your Agent configuration file or environment variables."
	if configPath != "" {
		where = "Check the settings listed below in " + inlineCode(configPath) + " or in environment variables."
	}
	extra, err := structpb.NewStruct(map[string]any{contextKeyConfigPath: configPath, contextKeyConversions: details})
	if err != nil {
		return nil, err
	}
	return &healthplatform.Issue{
		IssueName: issueName, IssueType: issueType,
		Title:       fmt.Sprintf("%s converted %d configuration %s", name, len(conversions), english.PluralWord(len(conversions), "type", "types")),
		Description: strings.Join(descriptions, " "), Category: "configuration", Location: component, Source: component,
		Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW, Extra: extra, Tags: []string{"config", "type-conversion"},
		Remediation: &healthplatform.Remediation{Summary: "Use the expected types in your configuration, then restart " + strings.ToLower(name) + ".",
			Steps: []*healthplatform.RemediationStep{{Order: 1, Text: where}, {Order: 2, Text: strings.Join(fixes, "\n")}, {Order: 3, Text: "Restart " + strings.ToLower(name) + "."}},
		},
	}, nil
}
