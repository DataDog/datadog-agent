// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

// isEligible returns whether a worker advertising the given compatibility
// may run a check with the given name. nil compat means unrestricted.
func isEligible(compat *types.CheckCompatibility, checkName string) bool {
	if compat == nil {
		return true
	}
	if len(compat.Include) > 0 && !slices.Contains(compat.Include, checkName) {
		return false
	}
	if slices.Contains(compat.Exclude, checkName) {
		return false
	}
	return true
}
