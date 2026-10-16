// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
)

const (
	workloadTypeAzureAppService = "azure_app_service"
	workloadTypeAzureFunction   = "azure_function"
)

func (l *LocalService) CanCollectInventory() bool { return false }
func (l *LocalService) GetInventoryData() serverlessdef.ServerlessInitData {
	return serverlessdef.ServerlessInitData{}
}

func (c *CloudRun) CanCollectInventory() bool { return false }
func (c *CloudRun) GetInventoryData() serverlessdef.ServerlessInitData {
	return serverlessdef.ServerlessInitData{}
}

func (c *CloudRunJobs) CanCollectInventory() bool { return false }
func (c *CloudRunJobs) GetInventoryData() serverlessdef.ServerlessInitData {
	return serverlessdef.ServerlessInitData{}
}

func (c *ContainerApp) CanCollectInventory() bool { return false }
func (c *ContainerApp) GetInventoryData() serverlessdef.ServerlessInitData {
	return serverlessdef.ServerlessInitData{}
}

func (m *MicroVM) CanCollectInventory() bool { return false }
func (m *MicroVM) GetInventoryData() serverlessdef.ServerlessInitData {
	return serverlessdef.ServerlessInitData{}
}
