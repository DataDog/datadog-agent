// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	serverlessinit "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
)

const (
	workloadTypeAzureAppService = "azure_app_service"
	workloadTypeAzureFunction   = "azure_function"
)

func (l *LocalService) CanCollectInventory() bool { return true }
func (l *LocalService) GetInventoryData() serverlessinit.InventoryData {
	return serverlessinit.InventoryData{}
}

func (c *CloudRun) CanCollectInventory() bool { return false }
func (c *CloudRun) GetInventoryData() serverlessinit.InventoryData {
	return serverlessinit.InventoryData{}
}

func (c *CloudRunJobs) CanCollectInventory() bool { return false }
func (c *CloudRunJobs) GetInventoryData() serverlessinit.InventoryData {
	return serverlessinit.InventoryData{}
}

func (c *ContainerApp) CanCollectInventory() bool { return false }
func (c *ContainerApp) GetInventoryData() serverlessinit.InventoryData {
	return serverlessinit.InventoryData{}
}

func (m *MicroVM) CanCollectInventory() bool { return false }
func (m *MicroVM) GetInventoryData() serverlessinit.InventoryData {
	return serverlessinit.InventoryData{}
}
