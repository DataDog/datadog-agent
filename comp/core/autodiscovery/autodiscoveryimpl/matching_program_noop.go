// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cel

package autodiscoveryimpl

import (
	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

func createMatchingProgram(_ workloadfilter.Rules) (program integration.MatchingProgram, celADID adtypes.CelIdentifier, compileErr error, recError error) {
	return nil, "", nil, nil
}
