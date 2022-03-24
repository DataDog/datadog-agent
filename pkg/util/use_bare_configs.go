// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import "github.com/DataDog/datadog-agent/pkg/config"

// set in testing to force the result of CcaInAD
var forcedCcaInAD *bool

// ForceCcaInAD forces a choice for CcaInAD, and returns
// a function to revert the change, suitable for use in a `defer`.
func ForceCcaInAD(use bool) func() {
	forcedCcaInAD = &use
	return func() {
		forcedCcaInAD = nil
	}
}

// CcaInAD returns the value of the logs_config.cca_in_ad
// feature flag.  This is temporary, as this functionality will eventually be
// the only option.
func CcaInAD() bool {
	if forcedCcaInAD != nil {
		return *forcedCcaInAD
	}

	return config.Datadog.GetBool("logs_config.cca_in_ad")
}
