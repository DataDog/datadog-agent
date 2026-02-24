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
	"time"

	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	utilsort "github.com/DataDog/datadog-agent/pkg/util/sort"

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
func DetectCloudProvider(ctx context.Context, collectAccountID bool) (string, string) {
	for _, cloudDetector := range cloudProviderDetectors {
		if cloudDetector.callback(ctx) {
			log.Infof("Cloud provider %s detected", cloudDetector.name)

			// fetch the account ID for this cloud provider
			if collectAccountID && cloudDetector.accountIDCallback != nil {
				accountID, err := cloudDetector.accountIDCallback(ctx)
				if err != nil {
					log.Debugf("Could not detect cloud provider account ID: %v", err)
				} else if accountID != "" {
					log.Infof("Detecting cloud provider account ID from %s: %+q", cloudDetector.name, accountID)
					return cloudDetector.name, accountID
				}
			}
			return cloudDetector.name, ""
		}
	}
	log.Info("No cloud provider detected")
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
	name       string
	isCloudEnv bool
	callback   func(context.Context) ([]string, error)
}

// getValidHostAliases is an alias from pkg config
func getValidHostAliases(_ context.Context) ([]string, error) {
	aliases := []string{}
	for _, alias := range configsetup.Datadog().GetStringSlice("host_aliases") {
		if err := validate.ValidHostname(alias); err == nil {
			aliases = append(aliases, alias)
		} else {
			log.Warnf("skipping invalid host alias '%s': %s", alias, err)
		}
	}

	return aliases, nil
}

var hostAliasesDetectors = []cloudProviderAliasesDetector{
	{name: "config", callback: getValidHostAliases},
	{name: alibaba.CloudProviderName, isCloudEnv: true, callback: alibaba.GetHostAliases},
	{name: ec2.CloudProviderName, isCloudEnv: true, callback: ec2.GetHostAliases},
	{name: azure.CloudProviderName, isCloudEnv: true, callback: azure.GetHostAliases},
	{name: gce.CloudProviderName, isCloudEnv: true, callback: gce.GetHostAliases},
	{name: cloudfoundry.CloudProviderName, isCloudEnv: true, callback: cloudfoundry.GetHostAliases},
	{name: "kubelet", callback: kubelet.GetHostAliases},
	{name: tencent.CloudProviderName, isCloudEnv: true, callback: tencent.GetHostAliases},
	{name: oracle.CloudProviderName, isCloudEnv: true, callback: oracle.GetHostAliases},
	{name: ibm.CloudProviderName, isCloudEnv: true, callback: ibm.GetHostAliases},
	{name: kubernetes.CloudProviderName, callback: kubernetes.GetHostAliases},
}

var (
	hostAliasMutex   = sync.Mutex{}
	hostAliasLogOnce = true
)

// GetHostAliases returns the hostname aliases and the name of the possible cloud providers
func GetHostAliases(ctx context.Context) ([]string, string) {
	aliases := []string{}
	cloudprovider := ""

	// cloud providers endpoints can take a few seconds to answer. We're using a WaitGroup to call all of them
	// concurrently since GetHostAliases is called during the agent startup and is blocking.
	var wg sync.WaitGroup

	for _, hostAliasesDetector := range hostAliasesDetectors {
		wg.Add(1)
		go func(hostAliasesDetector cloudProviderAliasesDetector) {
			defer wg.Done()

			cloudAliases, err := hostAliasesDetector.callback(ctx)
			if err != nil {
				log.Debugf("No %s Host Alias: %s", hostAliasesDetector.name, err)
			} else if len(cloudAliases) > 0 {
				hostAliasMutex.Lock()
				aliases = append(aliases, cloudAliases...)
				if hostAliasesDetector.isCloudEnv {
					if cloudprovider == "" {
						cloudprovider = hostAliasesDetector.name
					} else if hostAliasLogOnce {
						log.Warnf("Ambiguous cloud provider: %s or %s", cloudprovider, hostAliasesDetector.name)
						hostAliasLogOnce = false
					}
				}
				hostAliasMutex.Unlock()
			}
		}(hostAliasesDetector)
	}
	wg.Wait()

	return utilsort.UniqInPlace(aliases), cloudprovider
}

type cloudProviderCCRIDDetector func(context.Context) (string, error)

var hostCCRIDDetectors = map[string]cloudProviderCCRIDDetector{
	azure.CloudProviderName:  azure.GetHostCCRID,
	ec2.CloudProviderName:    ec2.GetHostCCRID,
	gce.CloudProviderName:    gce.GetHostCCRID,
	oracle.CloudProviderName: oracle.GetHostCCRID,
}

// GetHostCCRID returns the host CCRID from the first provider that works
func GetHostCCRID(ctx context.Context, detectedCloud string) string {
	if detectedCloud == "" {
		log.Infof("No Host CCRID, no cloudprovider detected")
		return ""
	}

	// Try the cloud that was previously detected
	if callback, found := hostCCRIDDetectors[detectedCloud]; found {
		hostCCRID, err := callback(ctx)
		if err != nil {
			log.Debugf("Could not fetch %s Host CCRID: %s", detectedCloud, err)
			return ""
		}
		return hostCCRID
	}
	// When running in k8s, kubelet may be detected by GetHostAliases (this is
	// non-deterministic). For such cases, we try each of the possible CCRID
	// cloud providers that we know about.
	var wg sync.WaitGroup
	m := sync.Mutex{}
	hostCCRID := ""

	// Call each cloud provider concurrently, since this is called during startup
	for _, ccridDetector := range hostCCRIDDetectors {
		wg.Add(1)
		go func(ccridDetector cloudProviderCCRIDDetector) {
			defer wg.Done()

			ccrid, err := ccridDetector(ctx)
			if err == nil {
				m.Lock()
				hostCCRID = ccrid
				m.Unlock()
			}
		}(ccridDetector)
	}
	wg.Wait()

	if hostCCRID == "" {
		log.Infof("No Host CCRID found for cloudprovider: %q", detectedCloud)
	}
	return hostCCRID
}

type cloudProviderInstanceTypeDetector func(context.Context) (string, error)

var hostInstanceTypeDetectors = map[string]cloudProviderInstanceTypeDetector{
	ec2.CloudProviderName:    ec2.GetInstanceType,
	gce.CloudProviderName:    gce.GetInstanceType,
	oracle.CloudProviderName: oracle.GetInstanceType,
	azure.CloudProviderName:  azure.GetInstanceType,
}

// GetInstanceType returns the instance type from the first cloud provider that works.
func GetInstanceType(ctx context.Context, detectedCloud string) string {
	if detectedCloud == "" {
		log.Infof("No instance type detected, no cloud provider detected")
		return ""
	}

	if callback, found := hostInstanceTypeDetectors[detectedCloud]; found {
		instanceType, err := callback(ctx)
		if err != nil {
			log.Infof("Could not fetch instance type for %s: %s", detectedCloud, err)
			return ""
		}
		return instanceType
	}

	log.Debugf("getting instance type from cloud provider %q is not supported", detectedCloud)
	return ""
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

type cloudProviderPreemptionDetector func(context.Context) (time.Time, error)

var preemptionDetectors = map[string]cloudProviderPreemptionDetector{
	ec2.CloudProviderName: ec2.GetSpotTerminationTime,
}

// ErrNotPreemptible is returned when the instance is not a preemptible instance
// (e.g., not an AWS Spot instance, not a GCE Preemptible instance).
// When this error is returned, callers should stop polling for preemption events.
var ErrNotPreemptible = errors.New("instance is not preemptible")

// ErrPreemptionUnsupported is returned when preemption detection is not supported
// for the given cloud provider.
var ErrPreemptionUnsupported = errors.New("preemption detection not supported for this cloud provider")

// GetPreemptionTerminationTime returns the scheduled termination time for a preemptible instance
// (e.g., AWS Spot, GCE Preemptible, Azure Spot).
// Returns ErrNotPreemptible if the instance is not preemptible.
// Returns ErrPreemptionUnsupported if the cloud provider doesn't support preemption detection.
// For now only EC2 is supported.
func GetPreemptionTerminationTime(ctx context.Context, cloudProviderName string) (time.Time, error) {
	callback, found := preemptionDetectors[cloudProviderName]
	if !found {
		return time.Time{}, ErrPreemptionUnsupported
	}

	terminationTime, err := callback(ctx)
	if err != nil {
		// Map cloud-provider-specific errors to generic errors
		if errors.Is(err, ec2.ErrNotSpotInstance) {
			return time.Time{}, ErrNotPreemptible
		}
		return time.Time{}, err
	}
	return terminationTime, nil
}
