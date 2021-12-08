// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudproviders

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/util"
	ecscommon "github.com/DataDog/datadog-agent/pkg/util/ecs/common"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"

	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/alibaba"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/tencent"
)

type cloudProviderDetector struct {
	name     string
	callback func(context.Context) bool
}

type cloudProviderNTPDetector struct {
	name     string
	callback func(context.Context) []string
}

// DetectCloudProvider detects the cloud provider where the agent is running in order:
func DetectCloudProvider(ctx context.Context) {
	detectors := []cloudProviderDetector{
		{name: ecscommon.CloudProviderName, callback: ecs.IsRunningOn},
		{name: ec2.CloudProviderName, callback: ec2.IsRunningOn},
		{name: gce.CloudProviderName, callback: gce.IsRunningOn},
		{name: azure.CloudProviderName, callback: azure.IsRunningOn},
		{name: alibaba.CloudProviderName, callback: alibaba.IsRunningOn},
		{name: tencent.CloudProviderName, callback: tencent.IsRunningOn},
	}

	for _, cloudDetector := range detectors {
		if cloudDetector.callback(ctx) {
			inventories.SetAgentMetadata(inventories.AgentCloudProvider, cloudDetector.name)
			log.Infof("Cloud provider %s detected", cloudDetector.name)
			return
		}
	}
	log.Info("No cloud provider detected")
}

// GetCloudProviderNTPHosts detects the cloud provider where the agent is running in order and returns its NTP host name.
func GetCloudProviderNTPHosts(ctx context.Context) []string {
	detectors := []cloudProviderNTPDetector{
		{name: ecscommon.CloudProviderName, callback: ecs.GetNTPHosts},
		{name: ec2.CloudProviderName, callback: ec2.GetNTPHosts},
		{name: gce.CloudProviderName, callback: gce.GetNTPHosts},
		{name: azure.CloudProviderName, callback: azure.GetNTPHosts},
		{name: alibaba.CloudProviderName, callback: alibaba.GetNTPHosts},
		{name: tencent.CloudProviderName, callback: tencent.GetNTPHosts},
	}

	for _, cloudNTPDetector := range detectors {
		if cloudNTPServers := cloudNTPDetector.callback(ctx); cloudNTPServers != nil {
			log.Infof("Using NTP servers from %s cloud provider: %+q", cloudNTPDetector.name, cloudNTPServers)
			return cloudNTPServers
		}
	}

	return nil
}

// GetHostAliases returns the hostname aliases from different provider
func GetHostAliases(ctx context.Context) []string {
	aliases := config.GetValidHostAliases()

	alibabaAlias, err := alibaba.GetHostAlias(ctx)
	if err != nil {
		log.Debugf("no Alibaba Host Alias: %s", err)
	} else if alibabaAlias != "" {
		aliases = append(aliases, alibabaAlias)
	}

	azureAlias, err := azure.GetHostAlias(ctx)
	if err != nil {
		log.Debugf("no Azure Host Alias: %s", err)
	} else if azureAlias != "" {
		aliases = append(aliases, azureAlias)
	}

	gceAliases, err := gce.GetHostAliases(ctx)
	if err != nil {
		log.Debugf("no GCE Host Alias: %s", err)
	} else {
		aliases = append(aliases, gceAliases...)
	}

	cfAliases, err := cloudfoundry.GetHostAliases(ctx)
	if err != nil {
		log.Debugf("no Cloud Foundry Host Alias: %s", err)
	} else if cfAliases != nil {
		aliases = append(aliases, cfAliases...)
	}

	k8sAlias, err := kubelet.GetHostAlias(ctx)
	if err != nil {
		log.Debugf("no Kubernetes Host Alias (through kubelet API): %s", err)
	} else if k8sAlias != "" {
		aliases = append(aliases, k8sAlias)
	}

	k8sAliases, err := kubernetes.GetHostAliases(ctx)
	if err != nil {
		log.Debugf("no Kubernetes Host Alias (through kube API server): %s", err)
	} else if k8sAliases != nil {
		aliases = append(aliases, k8sAliases...)
	}

	tencentAlias, err := tencent.GetHostAlias(ctx)
	if err != nil {
		log.Debugf("no Tencent Host Alias: %s", err)
	} else if tencentAlias != "" {
		aliases = append(aliases, tencentAlias)
	}

	return util.SortUniqInPlace(aliases)
}
