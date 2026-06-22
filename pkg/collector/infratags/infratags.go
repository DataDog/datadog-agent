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

// tagForMode returns the infra_mode tag for the given infrastructure_mode value,
// or ("", false) if the mode does not trigger metric tagging.
// To add a new taggable mode, add a case here and register its
// integration.<mode>.tagged config key in pkg/config/setup.
func tagForMode(infraMode string) (tag string, ok bool) {
	switch infraMode {
	case "cloud_cost_only":
		return InfraModeCloudCostTag, true
	default:
		return "", false
	}
}

// IsTaggableMode reports whether the active infrastructure_mode triggers infra_mode metric tagging.
func IsTaggableMode(cfg pkgconfigmodel.Reader) bool {
	_, ok := tagForMode(cfg.GetString("infrastructure_mode"))
	return ok
}

const jmxDogstatsdCheckNameTagPrefix = "dd.internal.jmx_check_name:"

// AppendJMXDogstatsdInfraTags appends the infra_mode tag for the active infrastructure mode when
// rawTags include dd.internal.jmx_check_name and the JMX integration is eligible per IsTagged.
// cfg may be nil (no-op). Plain DogStatsD metrics (no JMX check tag) are unchanged.
func AppendJMXDogstatsdInfraTags(tags []string, rawTags []string, cfg pkgconfigmodel.Reader) []string {
	if cfg == nil {
		return tags
	}
	jmxCheckName := ""
	for _, t := range rawTags {
		if strings.HasPrefix(t, jmxDogstatsdCheckNameTagPrefix) {
			jmxCheckName = t[len(jmxDogstatsdCheckNameTagPrefix):]
			break
		}
	}
	if jmxCheckName == "" || !IsTagged(jmxCheckName, cfg) {
		return tags
	}
	infraMode := cfg.GetString("infrastructure_mode")
	tag, ok := tagForMode(infraMode)
	if !ok {
		return tags
	}
	return append(tags, tag)
}

// IsTagged reports whether checkName should receive an infra_mode tag for the active infrastructure
// mode. Custom checks (names prefixed with custom_) are never tagged, matching the agent's notion
// of user-owned checks separate from packaged integrations.
func IsTagged(checkName string, cfg pkgconfigmodel.Reader) bool {
	infraMode := cfg.GetString("infrastructure_mode")
	if _, ok := tagForMode(infraMode); !ok {
		return false
	}
	if strings.HasPrefix(checkName, "custom_") {
		return false
	}

	taggedChecks := cfg.GetStringSlice("integration." + infraMode + ".tagged")
	if len(taggedChecks) == 0 {
		return true
	}
	return slices.Contains(taggedChecks, checkName)
}

type infraTagsAppender interface {
	AppendInfraTags(tags []string)
}

func appendInfraTags(s infraTagsAppender, cfg pkgconfigmodel.Reader) {
	infraMode := cfg.GetString("infrastructure_mode")
	tag, ok := tagForMode(infraMode)
	if !ok {
		return
	}
	s.AppendInfraTags([]string{tag})
}

// ApplySenderTags appends the infra_mode tag to the check sender's infra tags when the
// integration is eligible for tagging under the active infrastructure mode.
func ApplySenderTags(senderManager sender.SenderManager, id checkid.ID, integrationName string, cfg pkgconfigmodel.Reader) {
	if !IsTagged(integrationName, cfg) {
		return
	}
	s, err := senderManager.GetSender(id)
	if err != nil {
		log.Debugf("infra mode tags: skipping %s (%s): %v", integrationName, id, err)
		return
	}
	appendInfraTags(s, cfg)
}
