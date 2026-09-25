// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
)

// DMIChassisAssetTag is the chassis asset tag Oracle Cloud Infrastructure sets on both bare
// metal and virtual machine instances. Source:
// https://docs.oracle.com/en-us/iaas/Content/Compute/References/bare-metal-vm-differences.htm
const DMIChassisAssetTag = "OracleCloud.com"

// isChassisAssetTagOracle returns true if the DMI chassis asset tag identifies this host as
// Oracle Cloud Infrastructure.
func isChassisAssetTagOracle() bool {
	if !pkgconfigsetup.Datadog().GetBool("oracle_use_dmi") {
		return false
	}
	return strings.TrimSpace(dmi.GetChassisAssetTag()) == DMIChassisAssetTag
}

// IsRunningOnDMI returns true if DMI information identifies this host as Oracle Cloud
// Infrastructure, without making any network call. This is faster than IsRunningOn, which
// falls back to querying the metadata endpoint.
func IsRunningOnDMI() bool {
	return isChassisAssetTagOracle()
}
