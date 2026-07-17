// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package invalidconfig reports datadog.yaml schema violations through the Agent Health Platform.
package invalidconfig

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/selfident"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// checker validates the merged in-memory config against the schema.
type checker struct {
	cfg       config.Component
	hostname  hostnameinterface.Component
	selfIdent *selfident.SelfIdent
}

func newChecker(cfg config.Component, hostname hostnameinterface.Component, selfIdent *selfident.SelfIdent) *checker {
	return &checker{cfg: cfg, hostname: hostname, selfIdent: selfIdent}
}

func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	return c.validate()
}

func (c *checker) validate() ([]runnerdef.IssueReport, error) {
	// AllSettingsWithoutDefaultOrSecrets returns only values the customer actually set
	raw := c.cfg.AllSettingsWithoutDefaultOrSecrets()
	if len(raw) == 0 {
		return nil, nil
	}
	normalized, err := normalizeForSchema(raw)
	if err != nil {
		return nil, fmt.Errorf("invalidconfig: normalize config: %w", err)
	}
	errs, schemaErr := schema.ValidateCoreConfig(normalized)
	if schemaErr != nil {
		pkglog.Warnf("invalidconfig: schema validator unavailable; skipping check: %v", schemaErr)
		return nil, schemaErr
	}
	if len(errs) == 0 {
		return nil, nil
	}
	return []runnerdef.IssueReport{
		{
			IssueID:   c.instanceIssueID(),
			IssueName: IssueName,
			Source:    "agent",
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

// instanceIssueID scopes IssueID to this agent's discriminator and config
// file. Without this, two hosts in the same org validating the same config
// file (or, on one host, the agent and cluster-agent validating their own
// distinct config files) would all report the bare IssueID: downstream
// aggregation keys recommendations on (org, IssueID) alone and would collapse
// them into a single case.
//
// The discriminator is this agent's owning DaemonSet uid when resolvable
// (selfIdent.IssueDiscriminator), so that a config file distributed by that
// DaemonSet to every node agent collapses into one case instead of one per
// host — a deliberate inversion of the default per-host scoping, since the
// underlying cause and fix are shared across the whole DaemonSet. It falls
// back to the hostname on non-Kubernetes agents, preserving today's per-host
// behavior there.
//
// Uses a 64-bit digest rather than 32-bit: at 32 bits, an org with ~10k
// distinct discriminator/config-path pairs would already have a ~1% chance
// of two of them colliding (birthday bound), silently recreating the exact
// aggregation bug this ID scoping exists to fix. At 64 bits that probability
// is ~2.7e-12 at the same fleet size — negligible at any realistic scale.
func (c *checker) instanceIssueID() string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%s", c.selfIdent.IssueDiscriminator(c.hostname.GetSafe(context.Background())), c.cfg.ConfigFileUsed())
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
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
