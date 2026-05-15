// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ccmtags applies CCM mode metadata via the check sender without mutating
// integration.Config (which would break autodiscovery digest alignment).
//
// Production path: CheckScheduler.getChecks calls ApplySenderTags after a successful loader.Load.
package ccmtags

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// IsTagged reports whether integrationName should receive the ccm_mode tag for the current config.
func IsTagged(checkName string, cfg pkgconfigmodel.Reader) bool {
	ccmMode := cfg.GetString("ccm_mode")
	if ccmMode == "" {
		return false
	}

	taggedChecks := cfg.GetStringSlice("integration.ccm_" + ccmMode + ".tagged")
	if len(taggedChecks) == 0 {
		return true
	}
	return slices.Contains(taggedChecks, checkName)
}

// ApplySenderTags appends ccm_mode:<value> to the check sender's infra tags when the
// integration is eligible for CCM tagging.
func ApplySenderTags(senderManager sender.SenderManager, id checkid.ID, integrationName string, cfg pkgconfigmodel.Reader) {
	if !IsTagged(integrationName, cfg) {
		return
	}
	s, err := senderManager.GetSender(id)
	if err != nil {
		log.Debugf("CCM mode tags: skipping %s (%s): %v", integrationName, id, err)
		return
	}
	ccmTag := "ccm_mode:" + cfg.GetString("ccm_mode")
	s.AppendInfraTags([]string{ccmTag})
}
