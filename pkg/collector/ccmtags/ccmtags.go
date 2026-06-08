// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ccmtags applies CCM mode metadata via the check sender without mutating
// integration.Config (which would break autodiscovery digest alignment).
//
// Production paths:
//   - CheckScheduler.getChecks calls ApplySenderTags after a successful loader.Load.
//   - DogStatsD server enriches JMX metrics (dd.internal.jmx_check_name) when the JMX check
//     is eligible; custom checks (custom_*) and plain DogStatsD metrics are not tagged.
package ccmtags

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InfraModeCloudCostTag is the tag appended to eligible integration metrics in cloud_cost_only mode.
const InfraModeCloudCostTag = "infrastructure_mode:cloud_cost_only"

const jmxDogstatsdCheckNameTagPrefix = "dd.internal.jmx_check_name:"

// AppendJMXDogstatsdCCMTags appends InfraModeCloudCostTag when rawTags include dd.internal.jmx_check_name
// and the JMX integration is eligible per IsTagged. cfg may be nil (no-op). Plain DogStatsD metrics
// (no JMX check tag) are unchanged.
func AppendJMXDogstatsdCCMTags(tags []string, rawTags []string, cfg pkgconfigmodel.Reader) []string {
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
	return append(tags, InfraModeCloudCostTag)
}

// IsTagged reports whether checkName should receive the infrastructure_mode tag for the current config.
// Custom checks (names prefixed with custom_) are never tagged, matching the agent's notion of
// user-owned checks separate from packaged integrations.
func IsTagged(checkName string, cfg pkgconfigmodel.Reader) bool {
	infraMode := cfg.GetString("infrastructure_mode")
	if infraMode != "cloud_cost_only" {
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

func appendCCMInfraTags(s infraTagsAppender, cfg pkgconfigmodel.Reader) {
	if cfg.GetString("infrastructure_mode") == "cloud_cost_only" {
		s.AppendInfraTags([]string{InfraModeCloudCostTag})
	}
}

// ApplySenderTags appends infrastructure_mode:<value> to the check sender's infra tags when the
// integration is eligible for CCM mode tagging.
func ApplySenderTags(senderManager sender.SenderManager, id checkid.ID, integrationName string, cfg pkgconfigmodel.Reader) {
	if !IsTagged(integrationName, cfg) {
		return
	}
	s, err := senderManager.GetSender(id)
	if err != nil {
		log.Debugf("CCM mode tags: skipping %s (%s): %v", integrationName, id, err)
		return
	}
	appendCCMInfraTags(s, cfg)
}
