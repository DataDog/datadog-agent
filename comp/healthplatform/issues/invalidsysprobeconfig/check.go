// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidsysprobeconfig

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"

	"go.yaml.in/yaml/v3"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/selfident"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
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

// instanceIssueID scopes IssueID to this agent's discriminator (the owning
// DaemonSet's uid when resolvable, else the hostname) and config file, so the
// recommendations service (which keys on orgID + issueID, ignoring hostname)
// collapses cluster-distributed template violations into one case instead of
// one per host, while still keeping distinct config files distinct.
func (c *checker) instanceIssueID() string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%s", c.selfIdent.IssueDiscriminator(c.hostname.GetSafe(context.Background())), c.cfg.ConfigFileUsed())
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
	errs, schemaErr := schema.ValidateSystemProbeConfig(normalized)
	if schemaErr != nil {
		pkglog.Warnf("invalidsysprobeconfig: schema validator unavailable; skipping check: %v", schemaErr)
		return nil, schemaErr
	}
	if len(errs) == 0 {
		return nil, nil
	}
	return []runnerdef.IssueReport{
		{
			IssueID:   c.instanceIssueID(),
			IssueName: IssueName,
			Source:    "system-probe",
			Context: func() map[string]string {
				ctx := map[string]string{
					contextKeyConfigPath: c.cfg.ConfigFileUsed(),
					contextKeyErrorCount: strconv.Itoa(len(errs)),
				}
				for i, e := range errs {
					ctx[contextErrorKey(i)] = e
				}
				return ctx
			}(),
		},
	}, nil
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

// normalizeForSchema coerces a Go-native config map into JSON-native types via
// a YAML round-trip. ScrubYaml strips any accidental secret-like values
func normalizeForSchema(in map[string]any) (map[string]any, error) {
	b, err := yaml.Marshal(in)
	if err != nil {
		return nil, err
	}
	scrubbed, err := scrubber.ScrubYaml(b)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(scrubbed, &out); err != nil {
		return nil, err
	}
	return out, nil
}
