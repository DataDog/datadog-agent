// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cel

package integration

import (
	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// CreateMatchingPrograms is a no-op when CEL is not enabled
func CreateMatchingPrograms(_ workloadfilter.Rules, _ bool) (map[workloadfilter.ResourceType]MatchingProgram, []adtypes.CelIdentifier, error) {
	return nil, nil, nil
}
