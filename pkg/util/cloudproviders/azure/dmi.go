// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azure

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
)

func isBoardVendorAzureVM() bool {
	if !config.Datadog.GetBool("azure_vm_use_dmi") {
		return false
	}
	return dmi.GetBoardVendor() == DMIBoardVendor
}

// getAzureVMIDFromDMI fetches the Azure VM id for current host from DMI
//
// On Azure VM instances (older than 2014-09-18) dmi information contains the Azure VM Unique ID for the host. We check that the board vendor is
// Microsoft Corporation before using it. Source : https://azure.microsoft.com/en-us/blog/accessing-and-using-azure-vm-unique-id/
func getAzureVMIDFromDMI() ([]string, error) {
	// we don't want to collect anything on Fargate
	if fargate.IsFargateInstance() {
		return []string{}, fmt.Errorf("host alias detection through DMI is disabled on Fargate")
	}

	if !config.Datadog.GetBool("azure_vm_use_dmi") {
		return []string{}, fmt.Errorf("'azure_vm_use_dmi' is disabled")
	}

	if !isBoardVendorAzureVM() {
		return []string{}, fmt.Errorf("board vendor is not Microsoft Corporation")
	}

	productUUID := dmi.GetProductUUID()
	return []string{strings.ToLower(productUUID)}, nil
}
