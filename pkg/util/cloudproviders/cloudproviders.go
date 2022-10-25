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
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"

	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/alibaba"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/ibm"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/oracle"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/tencent"
)

type cloudProviderDetector struct {
	name     string
	callback func(context.Context) bool
}

type cloudProviderIdentifiersDetector struct {
	name     string
	callback func(context.Context) map[string]string
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
		{name: oracle.CloudProviderName, callback: oracle.IsRunningOn},
		{name: ibm.CloudProviderName, callback: ibm.IsRunningOn},
	}

	for _, cloudDetector := range detectors {
		if cloudDetector.callback(ctx) {
			inventories.SetAgentMetadata(inventories.AgentCloudProvider, cloudDetector.name)
			log.Infof("Cloud provider %s detected", cloudDetector.name)
			return
		}
	}
	log.Info("No cloud provider detected")

	// At this point we haven't found any cloud provider. Some cloud provider aren't metadata endpoint aren't
	// reliable and some customer setup prevent us from detecting cloud providers. This can prevent the backend from
	// associating the hostname found by the agent to a known host in the backend.
	//
	// In those cases, we gather as much data, or hint, as possible on the cloud providers. This will help corrolate
	// hosts in the backend.

	// not all cloud providers provides hints yet.
	identifiersDetectors := []cloudProviderIdentifiersDetector{
		{name: ec2.CloudProviderName, callback: ec2.CollectIdentifiers},
	}

	cloudIdentifiers := map[string]string{}
	for _, cloudDetector := range identifiersDetectors {
		for identifier, value := range cloudDetector.callback(ctx) {
			cloudIdentifiers[identifier] = value
			log.Infof("Identifier for cloud provider %s detected: '%s' is '%s'", cloudDetector.name, identifier, value)
		}
	}

	if len(cloudIdentifiers) != 0 {
		inventories.SetHostMetadata(inventories.HostCloudIdentifiers, cloudIdentifiers)
	}
}

type cloudProviderNTPDetector struct {
	name     string
	callback func(context.Context) []string
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
		{name: oracle.CloudProviderName, callback: oracle.GetNTPHosts},
	}

	for _, cloudNTPDetector := range detectors {
		if cloudNTPServers := cloudNTPDetector.callback(ctx); cloudNTPServers != nil {
			log.Infof("Using NTP servers from %s cloud provider: %+q", cloudNTPDetector.name, cloudNTPServers)
			return cloudNTPServers
		}
	}

	return nil
}

type cloudProviderAliasesDetector struct {
	name     string
	callback func(context.Context) ([]string, error)
}

// GetHostAliases returns the hostname aliases from different provider
func GetHostAliases(ctx context.Context) []string {
	aliases := config.GetValidHostAliases()

	detectors := []cloudProviderAliasesDetector{
		{name: alibaba.CloudProviderName, callback: alibaba.GetHostAliases},
		{name: ec2.CloudProviderName, callback: ec2.GetHostAliases},
		{name: azure.CloudProviderName, callback: azure.GetHostAliases},
		{name: gce.CloudProviderName, callback: gce.GetHostAliases},
		{name: cloudfoundry.CloudProviderName, callback: cloudfoundry.GetHostAliases},
		{name: "kubelet", callback: kubelet.GetHostAliases},
		{name: tencent.CloudProviderName, callback: tencent.GetHostAliases},
		{name: oracle.CloudProviderName, callback: oracle.GetHostAliases},
		{name: ibm.CloudProviderName, callback: ibm.GetHostAliases},
		{name: kubernetes.CloudProviderName, callback: kubernetes.GetHostAliases},
	}

	for _, cloudAliasesDetector := range detectors {
		cloudAliases, err := cloudAliasesDetector.callback(ctx)
		if err != nil {
			log.Debugf("no %s Host Alias: %s", cloudAliasesDetector.name, err)
		} else if len(cloudAliases) > 0 {
			aliases = append(aliases, cloudAliases...)
		}
	}

	return util.SortUniqInPlace(aliases)
}
