// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package names

import utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"

// shouldDropCloudCost applies cloud_cost_only filtering: forward metrics that bypass the
// allowlist and metrics on the integration allowlist; drop other integration metrics.
func shouldDropCloudCost(ctx FilterContext, blockList, allowList utilstrings.Matcher, additionalChecks []string) bool {
	if blockList.ShouldDrop(ctx.Name) {
		return true
	}
	if ctx.BypassesCloudCostFilter(additionalChecks) {
		return false
	}
	if !allowList.ShouldDrop(ctx.Name) {
		return false
	}
	return true
}
