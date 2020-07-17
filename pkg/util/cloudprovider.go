// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package util

import (
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/util/alibaba"
	"github.com/DataDog/datadog-agent/pkg/util/azure"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/tencent"
)

type cloudProviderDetector struct {
	name     string
	callback func() bool
}

// DetectCloudProvider detects the cloud provider where the agent is running in order:
// * AWS ECS/Fargate
// * AWS EC2
// * GCE
// * Azure
// * Alibaba
// * Tencent
func DetectCloudProvider() {
	detectors := []cloudProviderDetector{
		{name: ecs.CloudProviderName, callback: ecs.IsRunningOn},
		{name: ec2.CloudProviderName, callback: ec2.IsRunningOn},
		{name: gce.CloudProviderName, callback: gce.IsRunningOn},
		{name: azure.CloudProviderName, callback: azure.IsRunningOn},
		{name: alibaba.CloudProviderName, callback: alibaba.IsRunningOn},
		{name: tencent.CloudProviderName, callback: tencent.IsRunningOn},
	}

	for _, cloudDetector := range detectors {
		if cloudDetector.callback() {
			inventories.SetAgentMetadata(inventories.CloudProviderMetatadaName, cloudDetector.name)
			log.Infof("Cloud provider %s detected", cloudDetector.name)
			return
		}
	}
	log.Info("No cloud provider detected")
}
