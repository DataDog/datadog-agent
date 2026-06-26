// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infratags applies infrastructure mode metadata via the check sender without mutating
// integration.Config (which would break autodiscovery digest alignment).
//
// Production paths:
//   - CheckScheduler.getChecks calls ApplySenderTags after a successful loader.Load.
//   - DogStatsD server enriches JMX metrics (dd.internal.jmx_check_name) when the JMX check
//     is eligible; custom checks (custom_*) and plain DogStatsD metrics are not tagged.
package infratags

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InfraModeCloudCostTag is the tag appended to eligible integration metrics in cloud_cost_only mode.
const InfraModeCloudCostTag = "infra_mode:cloud_cost_only"

// tagsForMode returns the infra_mode tags for the given infrastructure_mode value,
// or (nil, false) if the mode does not trigger metric tagging.
// To add a new taggable mode, add a case here and register its
// integration.<mode>.tagged config key in pkg/config/setup.
func tagsForMode(infraMode string) (tags []string, ok bool) {
	switch infraMode {
	case "cloud_cost_only":
		return []string{InfraModeCloudCostTag}, true
	default:
		return nil, false
	}
}

// ResolveEnrichmentState resolves the infra_mode tags and tagged-checks allow-list from cfg, to be
// stored once at startup and passed to AppendJMXDogstatsdInfraTags on the hot path.
// Returns (nil, nil) if the active infrastructure_mode does not trigger tagging.
func ResolveEnrichmentState(cfg pkgconfigmodel.Reader) (infraModeTags []string, taggedChecks []string) {
	infraMode := cfg.GetString("infrastructure_mode")
	tags, ok := tagsForMode(infraMode)
	if !ok {
		return nil, nil
	}
	return tags, cfg.GetStringSlice("integration." + infraMode + ".tagged")
}

// IsTagged reports whether checkName should receive infra_mode tags given a
// pre-resolved allow-list. Custom checks (custom_ prefix) are always excluded.
// An empty taggedChecks means all non-custom checks are eligible.
func IsTagged(checkName string, taggedChecks []string) bool {
	if checkName == "" || strings.HasPrefix(checkName, "custom_") {
		return false
	}
	return len(taggedChecks) == 0 || slices.Contains(taggedChecks, checkName)
}

// AppendJMXDogstatsdInfraTags appends the pre-resolved infra_mode tags when jmxCheckName is
// eligible. infraModeTags and taggedChecks must be resolved once at startup via
// ResolveEnrichmentState. Zero config reads and zero allocations on the non-tagging path.
func AppendJMXDogstatsdInfraTags(tags []string, jmxCheckName string, infraModeTags []string, taggedChecks []string) []string {
	if len(infraModeTags) == 0 || !IsTagged(jmxCheckName, taggedChecks) {
		return tags
	}
	return append(tags, infraModeTags...)
}

// ApplySenderTags appends the infra_mode tags to the check sender's infra tags when the
// integration is eligible for tagging under the active infrastructure mode.
func ApplySenderTags(senderManager sender.SenderManager, id checkid.ID, integrationName string, cfg pkgconfigmodel.Reader) {
	infraMode := cfg.GetString("infrastructure_mode")
	tags, ok := tagsForMode(infraMode)
	if !ok {
		return
	}
	if !IsTagged(integrationName, cfg.GetStringSlice("integration."+infraMode+".tagged")) {
		return
	}
	s, err := senderManager.GetSender(id)
	if err != nil {
		log.Debugf("infra mode tags: skipping %s (%s): %v", integrationName, id, err)
		return
	}
	s.AppendInfraTags(tags)
}
