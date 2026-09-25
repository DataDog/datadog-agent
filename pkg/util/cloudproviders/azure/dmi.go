// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azure

import (
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
)

// DMIChassisAssetTag is the chassis asset tag Azure sets on all VMs, regardless of
// the underlying hypervisor. Source:
// https://learn.microsoft.com/en-us/azure/virtual-machines/generation-2#generation-2-vm-detection
const DMIChassisAssetTag = "7783-7084-3265-9085-8269-3286-77"

// isChassisAssetTagAzure returns true if the DMI chassis asset tag identifies this host as Azure.
// This lets us know we're running on Azure without a network call to the metadata endpoint.
func isChassisAssetTagAzure() bool {
	if !pkgconfigsetup.Datadog().GetBool("azure_use_dmi") {
		return false
	}
	return strings.TrimSpace(dmi.GetChassisAssetTag()) == DMIChassisAssetTag
}

// IsRunningOnDMI returns true if DMI information identifies this host as Azure, without
// making any network call. This is faster than IsRunningOn, which falls back to
// querying the metadata endpoint.
func IsRunningOnDMI() bool {
	return isChassisAssetTagAzure()
}
