// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infratags applies infrastructure mode metadata via the check sender without mutating
// integration.Config (which would break autodiscovery digest alignment).
//
// Production paths:
//   - CheckScheduler.getChecks calls sender.SetInfraTagger after a successful loader.Load.
//   - DogStatsD server enriches JMX metrics (dd.internal.jmx_check_name) when the JMX check
//     is eligible; custom checks (custom_*) and plain DogStatsD metrics are not tagged.
//
// The tag value comes from configutils.MarkedInfraMode (same allowlist as payload marks).
// Metrics eligibility stays here: tagger.Component has no check name and must not gate series.
package infratags

import (
	"fmt"
	"slices"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

// InfraModeCloudCostTag is the historical constant for the CCM metrics mark.
// Prefer building the tag from configutils.InfraModeTagKey and MarkedInfraMode.
const InfraModeCloudCostTag = "infra_mode:cloud_cost_only"

// Tagger holds the pre-resolved infra mode tagging state for Agent integration metrics.
// A nil *Tagger disables tagging.
type Tagger struct {
	infraModeTags []string
	taggedChecks  map[string]struct{} // nil = all non-custom checks eligible
}

// NewTagger resolves the infra mode tagging configuration from cfg.
// Returns nil if the active infrastructure_mode does not trigger tagging.
func NewTagger(cfg pkgconfigmodel.Reader) *Tagger {
	mode := configutils.MarkedInfraMode(cfg)
	if mode == "" {
		return nil
	}
	tags := []string{fmt.Sprintf("%s:%s", configutils.InfraModeTagKey, mode)}

	checks := cfg.GetStringSlice("integration." + mode + ".tagged")
	if len(checks) == 0 {
		return &Tagger{infraModeTags: tags}
	}
	taggedChecks := make(map[string]struct{}, len(checks))
	for _, c := range checks {
		taggedChecks[c] = struct{}{}
	}
	return &Tagger{infraModeTags: tags, taggedChecks: taggedChecks}
}

// IsCheckEligible reports whether the given check should receive infra mode tags.
func (t *Tagger) IsCheckEligible(checkName string) bool {
	// nil = no infra mode tagging
	if t == nil {
		return false
	}
	// empty check name or custom check = never eligible
	if checkName == "" || strings.HasPrefix(checkName, "custom_") {
		return false
	}
	// empty taggedChecks = all non-custom checks eligible
	if t.taggedChecks == nil {
		return true
	}
	_, ok := t.taggedChecks[checkName]
	return ok
}

// AppendTags appends the pre-resolved infra_mode tags, skipping values already present.
func (t *Tagger) AppendTags(tags []string) []string {
	if t == nil || len(t.infraModeTags) == 0 {
		return tags
	}
	for _, infraTag := range t.infraModeTags {
		if !slices.Contains(tags, infraTag) {
			tags = append(tags, infraTag)
		}
	}
	return tags
}
