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

// isEligible returns true if a worker advertising the given compatibility is
// allowed to run a check with the given name. A nil compat means the worker
// is unrestricted. If compat.Include is non-empty, checkName must appear in
// it; compat.Exclude is always subtracted afterward.
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
