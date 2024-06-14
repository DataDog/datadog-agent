// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cloudproviders provides utilities to detect the cloud provider.
package cloudproviders

import (
	"context"
	"errors"
	"sync"

	logComp "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/ec2"

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
	name              string
	callback          func(context.Context) bool
	accountIDCallback func(context.Context) (string, error)
}

var cloudProviderDetectors = []cloudProviderDetector{
	{name: ec2.CloudProviderName, callback: ec2.IsRunningOn, accountIDCallback: ec2.GetAccountID},
	{name: gce.CloudProviderName, callback: gce.IsRunningOn, accountIDCallback: gce.GetProjectID},
	{name: azure.CloudProviderName, callback: azure.IsRunningOn, accountIDCallback: azure.GetSubscriptionID},
	{name: alibaba.CloudProviderName, callback: alibaba.IsRunningOn},
	{name: tencent.CloudProviderName, callback: tencent.IsRunningOn},
	{name: oracle.CloudProviderName, callback: oracle.IsRunningOn},
	{name: ibm.CloudProviderName, callback: ibm.IsRunningOn},
}

// DetectCloudProvider detects the cloud provider where the agent is running in order:
func DetectCloudProvider(ctx context.Context, collectAccountID bool, l logComp.Component) (string, string) {
	for _, cloudDetector := range cloudProviderDetectors {
		if cloudDetector.callback(ctx) {
			l.Infof("Cloud provider %s detected", cloudDetector.name)

			// fetch the account ID for this cloud provider
			if collectAccountID && cloudDetector.accountIDCallback != nil {
				accountID, err := cloudDetector.accountIDCallback(ctx)
				if err != nil {
					l.Debugf("Could not detect cloud provider account ID: %v", err)
				} else if accountID != "" {
					l.Infof("Detecting cloud provider account ID from %s: %+q", cloudDetector.name, accountID)
					return cloudDetector.name, accountID
				}
			}
			return cloudDetector.name, ""
		}
	}
	l.Info("No cloud provider detected")
	return "", ""
}

type cloudProviderNTPDetector struct {
	name     string
	callback func(context.Context) []string
}

// GetCloudProviderNTPHosts detects the cloud provider where the agent is running in order and returns its NTP host name.
func GetCloudProviderNTPHosts(ctx context.Context) []string {
	detectors := []cloudProviderNTPDetector{
		{name: ec2.CloudProviderName, callback: ec2.GetNTPHosts},
		{name: gce.CloudProviderName, callback: gce.GetNTPHosts},
		{name: azure.CloudProviderName, callback: azure.GetNTPHosts},
		{name: alibaba.CloudProviderName, callback: alibaba.GetNTPHosts},
		{name: tencent.CloudProviderName, callback: tencent.GetNTPHosts},
		{name: oracle.CloudProviderName, callback: oracle.GetNTPHosts},
	}

	for _, cloudNTPDetector := range detectors {
		if cloudNTPServers := cloudNTPDetector.callback(ctx); cloudNTPServers != nil {
			log.Infof("Detected %s cloud provider environment with NTP server(s) at %+q", cloudNTPDetector.name, cloudNTPServers)
			return cloudNTPServers
		}
	}

	return nil
}

type cloudProviderAliasesDetector struct {
	name     string
	callback func(context.Context) ([]string, error)
}

var hostAliasesDetectors = []cloudProviderAliasesDetector{
	{name: "config", callback: config.GetValidHostAliases},
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

// GetHostAliases returns the hostname aliases from different provider
func GetHostAliases(ctx context.Context) []string {
	aliases := []string{}

	// cloud providers endpoints can take a few seconds to answer. We're using a WaitGroup to call all of them
	// concurrently since GetHostAliases is called during the agent startup and is blocking.
	var wg sync.WaitGroup
	m := sync.Mutex{}

	for _, cloudAliasesDetector := range hostAliasesDetectors {
		wg.Add(1)
		go func(cloudAliasesDetector cloudProviderAliasesDetector) {
			defer wg.Done()

			cloudAliases, err := cloudAliasesDetector.callback(ctx)
			if err != nil {
				log.Debugf("No %s Host Alias: %s", cloudAliasesDetector.name, err)
			} else if len(cloudAliases) > 0 {
				m.Lock()
				aliases = append(aliases, cloudAliases...)
				m.Unlock()
			}
		}(cloudAliasesDetector)
	}
	wg.Wait()

	return util.SortUniqInPlace(aliases)
}

// GetPublicIPv4 returns the public IPv4 from different providers
func GetPublicIPv4(ctx context.Context) (string, error) {
	publicIPProvider := map[string]func(context.Context) (string, error){
		ec2.CloudProviderName:   ec2.GetPublicIPv4,
		gce.CloudProviderName:   gce.GetPublicIPv4,
		azure.CloudProviderName: azure.GetPublicIPv4,
	}
	for name, fetcher := range publicIPProvider {
		publicIPv4, err := fetcher(ctx)
		if err == nil {
			log.Debugf("%s public IP: %s", name, publicIPv4)
			return publicIPv4, nil
		}
		log.Debugf("Could not fetch %s public IPv4: %s", name, err)
	}
	log.Infof("No public IPv4 address found")
	return "", errors.New("No public IPv4 address found")
}

var sourceDetectors = map[string]func() string{
	ec2.CloudProviderName: ec2.GetSourceName,
}

// GetSource returns the source used to pull information from the current cloud provider. For now only EC2 is
// supported. Example of sources for EC2: "IMDSv1", "IMDSv2", "DMI", ...
func GetSource(cloudProviderName string) string {
	if callback, ok := sourceDetectors[cloudProviderName]; ok {
		return callback()
	}
	return ""
}

var hostIDDetectors = map[string]func(context.Context) string{
	ec2.CloudProviderName: ec2.GetHostID,
}

// GetHostID returns the ID for a cloud provider for the current host. The host ID is unique to the cloud provider and
// is different from the hostname. For now only EC2 is supported.
func GetHostID(ctx context.Context, cloudProviderName string) string {
	if callback, ok := hostIDDetectors[cloudProviderName]; ok {
		return callback(ctx)
	}
	return ""
}
