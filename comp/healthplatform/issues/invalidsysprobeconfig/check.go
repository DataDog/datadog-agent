// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidsysprobeconfig

import (
	"context"
	"fmt"
	"hash/fnv"

	"go.yaml.in/yaml/v3"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// checker validates the customer-provided system-probe config against the schema.
type checker struct {
	cfg       sysprobeconfig.Component
	hostname  hostnameinterface.Component
	selfIdent *selfident.SelfIdent
}

func newChecker(cfg sysprobeconfig.Component, hostname hostnameinterface.Component, selfIdent *selfident.SelfIdent) *checker {
	return &checker{cfg: cfg, hostname: hostname, selfIdent: selfIdent}
}

// Scope each problem to its host or DaemonSet, config file, and setting.
func (c *checker) instanceIssueID(settingPath string) string {
	h := fnv.New64a()
	discriminator := issues.IssueDiscriminator(c.selfIdent, c.hostname.GetSafe(context.Background()))
	fmt.Fprintf(h, "%s\x00%s\x00%s", discriminator, c.cfg.ConfigFileUsed(), settingPath)
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
}

func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	return c.validate()
}

func (c *checker) validate() ([]runnerdef.IssueReport, error) {
	raw := customerConfig(c.cfg)
	if len(raw) == 0 {
		return nil, nil
	}
	normalized, err := normalizeForSchema(raw)
	if err != nil {
		return nil, fmt.Errorf("invalidsysprobeconfig: normalize config: %w", err)
	}
	violations, schemaErr := schema.ValidateSystemProbeConfigDetailed(normalized)
	if schemaErr != nil {
		pkglog.Warnf("invalidsysprobeconfig: schema validator unavailable; skipping check: %v", schemaErr)
		return nil, schemaErr
	}
	if len(violations) == 0 {
		return nil, nil
	}
	reports := make([]runnerdef.IssueReport, 0, len(violations))
	for _, violation := range violations {
		reports = append(reports, runnerdef.IssueReport{
			IssueID:   c.instanceIssueID(violation.Path),
			IssueName: IssueName,
			Source:    "system-probe",
			Context:   invalidconfig.BuildContext(c.cfg, c.cfg.ConfigFileUsed(), violation),
		})
	}
	return reports, nil
}

// customerConfig returns only the values the customer set in the system-probe config
// (file, env, CLI, ...etc). It merges the source layers in priority order, skipping defaults,
// secrets, and the agent-runtime layer that Adjust() writes to so the schema sees the
// customer's actual configuration, not values rewritten by post-load processing.
func customerConfig(cfg sysprobeconfig.Component) map[string]any {
	bySource := cfg.AllSettingsBySource()
	merged := map[string]any{}
	for _, src := range model.Sources { // ascending priority: higher layers win
		switch src {
		case model.SourceDefault, model.SourceSecret, model.SourceAgentRuntime:
			continue
		}
		if layer, ok := bySource[src].(map[string]any); ok {
			deepMerge(merged, layer)
		}
	}
	return merged
}

// deepMerge recursively merges src into dst: nested maps are merged, everything else overwrites.
func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			if dv, ok := dst[k].(map[string]any); ok {
				deepMerge(dv, sv)
				continue
			}
		}
		dst[k] = v
	}
}

// normalizeForSchema converts Go-native config types without changing the values.
func normalizeForSchema(in map[string]any) (map[string]any, error) {
	b, err := yaml.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
