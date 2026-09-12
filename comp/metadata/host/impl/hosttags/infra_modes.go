// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hosttags

const (
	infrastructureModeNone          = "none"
	infrastructureModeBasic         = "basic"
	infrastructureModeCloudCostOnly = "cloud_cost_only"
)

// simpleInfraModeTags maps infrastructure_mode to the infra_mode tag. end_user_device
// is handled separately in eudm.go, which collects extra hardware/OS tags.
// full is intentionally excluded: it's the default infrastructure_mode for
// every Agent install, so tagging it would add infra_mode:full to every host
// by default rather than only to hosts that opt into a specific mode.
var simpleInfraModeTags = map[string][]string{
	infrastructureModeNone:          {"infra_mode:" + infrastructureModeNone},
	infrastructureModeBasic:         {"infra_mode:" + infrastructureModeBasic},
	infrastructureModeCloudCostOnly: {"infra_mode:" + infrastructureModeCloudCostOnly},
}
