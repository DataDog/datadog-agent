// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// GetConfiguredTags returns list of tags from a configuration, based on
// `tags` (DD_TAGS) and `extra_tags“ (DD_EXTRA_TAGS), with `dogstatsd_tags` (DD_DOGSTATSD_TAGS)
// if includeDogdstatsd is true.
func GetConfiguredTags(c pkgconfigmodel.Reader, includeDogstatsd bool) []string {
	tags := c.GetStringSlice("tags")
	extraTags := c.GetStringSlice("extra_tags")

	var dsdTags []string
	if includeDogstatsd {
		dsdTags = c.GetStringSlice("dogstatsd_tags")
	}

	combined := make([]string, 0, len(tags)+len(extraTags)+len(dsdTags))
	combined = append(combined, tags...)
	combined = append(combined, extraTags...)
	combined = append(combined, dsdTags...)

	return combined
}

// GetConfiguredDCATags returns list of tags from a configuration, based on
// `cluster_checks.extra_tags` (DD_CLUSTER_CHECKS_EXTRA_TAGS) and
// `orchestrator_explorer.extra_tags (DD_ORCHESTRATOR_EXPLORER_EXTRA_TAGS).
func GetConfiguredDCATags(c pkgconfigmodel.Reader) []string {
	clusterCheckTags := c.GetStringSlice("cluster_checks.extra_tags")
	orchestratorTags := c.GetStringSlice("orchestrator_explorer.extra_tags")

	combined := make([]string, 0, len(clusterCheckTags)+len(orchestratorTags))
	combined = append(combined, clusterCheckTags...)
	combined = append(combined, orchestratorTags...)

	return combined
}
