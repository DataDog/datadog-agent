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
package infratags

import (
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// InfraModeCloudCostTag is the tag appended to eligible integration metrics in cloud_cost_only mode.
const InfraModeCloudCostTag = "infra_mode:cloud_cost_only"

// tagsForMode returns the infra_mode tags for the given infrastructure_mode value,
// or (nil, false) if the mode does not trigger metric tagging.
func tagsForMode(infraMode string) (tags []string, ok bool) {
	switch infraMode {
	case "cloud_cost_only":
		return []string{InfraModeCloudCostTag}, true
	default:
		return nil, false
	}
}

// Tagger holds the pre-resolved infra mode tagging state.
// A nil *Tagger disables tagging.
type Tagger struct {
	infraModeTags []string
	taggedChecks  map[string]struct{} // nil = all non-custom checks eligible
}

// NewTagger resolves the infra mode tagging configuration from cfg.
// Returns nil if the active infrastructure_mode does not trigger tagging.
func NewTagger(cfg pkgconfigmodel.Reader) *Tagger {
	infraMode := cfg.GetString("infrastructure_mode")
	tags, ok := tagsForMode(infraMode)
	if !ok {
		return nil
	}
	checks := cfg.GetStringSlice("integration." + infraMode + ".tagged")
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

// AppendTags appends the pre-resolved infra_mode tags.
func (t *Tagger) AppendTags(tags []string) []string {
	if t == nil || len(t.infraModeTags) == 0 {
		return tags
	}

	return append(tags, t.infraModeTags...)
}
