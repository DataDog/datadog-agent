// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gce

import (
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
)

// DMIProductName contains the DMI product name for GCE
const DMIProductName = "Google Compute Engine"

// isProductNameGCE returns true if the DMI product name identifies this host as GCE.
// This lets us know we're running on GCE without a network call to the metadata endpoint.
func isProductNameGCE() bool {
	if !pkgconfigsetup.Datadog().GetBool("gce_use_dmi") {
		return false
	}
	return strings.Contains(dmi.GetProductName(), DMIProductName)
}

// IsRunningOnDMI returns true if DMI information identifies this host as GCE, without
// making any network call. This is faster than IsRunningOn, which falls back to
// querying the metadata endpoint.
func IsRunningOnDMI() bool {
	return isProductNameGCE()
}
