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

func (l *LocalService) CanCollectMetadata() bool { return false }
func (l *LocalService) GetMetadata() serverlessdef.CloudServiceMetadata {
	return serverlessdef.CloudServiceMetadata{}
}

func (c *CloudRun) CanCollectMetadata() bool { return false }
func (c *CloudRun) GetMetadata() serverlessdef.CloudServiceMetadata {
	return serverlessdef.CloudServiceMetadata{}
}

func (c *CloudRunJobs) CanCollectMetadata() bool { return false }
func (c *CloudRunJobs) GetMetadata() serverlessdef.CloudServiceMetadata {
	return serverlessdef.CloudServiceMetadata{}
}

func (c *ContainerApp) CanCollectMetadata() bool { return false }
func (c *ContainerApp) GetMetadata() serverlessdef.CloudServiceMetadata {
	return serverlessdef.CloudServiceMetadata{}
}

func (m *MicroVM) CanCollectMetadata() bool { return false }
func (m *MicroVM) GetMetadata() serverlessdef.CloudServiceMetadata {
	return serverlessdef.CloudServiceMetadata{}
}
