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

	"github.com/DataDog/datadog-agent/pkg/config/helper"
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

var cloudProviderDetectors = map[string]cloudProviderDetector{
	ec2.CloudProviderName:     {name: ec2.CloudProviderName, callback: ec2.IsRunningOn, accountIDCallback: ec2.GetAccountID},
	gce.CloudProviderName:     {name: gce.CloudProviderName, callback: gce.IsRunningOn, accountIDCallback: gce.GetProjectID},
	azure.CloudProviderName:   {name: azure.CloudProviderName, callback: azure.IsRunningOn, accountIDCallback: azure.GetSubscriptionID},
	alibaba.CloudProviderName: {name: alibaba.CloudProviderName, callback: alibaba.IsRunningOn},
	tencent.CloudProviderName: {name: tencent.CloudProviderName, callback: tencent.IsRunningOn},
	oracle.CloudProviderName:  {name: oracle.CloudProviderName, callback: oracle.IsRunningOn},
	ibm.CloudProviderName:     {name: ibm.CloudProviderName, callback: ibm.IsRunningOn},
}

var cloudProviderDetectorResolutionOrder = []string{
	ec2.CloudProviderName,
	gce.CloudProviderName,
	azure.CloudProviderName,
	alibaba.CloudProviderName,
	tencent.CloudProviderName,
	oracle.CloudProviderName,
	ibm.CloudProviderName,
}

// DetectCloudProviderDMI detects the cloud provider using only DMI information, without making
// any network call. It only supports the cloud providers that implement DMI-based detection
// (EC2, GCE, Azure, Oracle Cloud); other providers require DetectCloudProvider's network calls.
func DetectCloudProviderDMI() string {
	switch {
	case ec2.IsRunningOnDMI():
		return ec2.CloudProviderName
	case gce.IsRunningOnDMI():
		return gce.CloudProviderName
	case azure.IsRunningOnDMI():
		return azure.CloudProviderName
	case oracle.IsRunningOnDMI():
		return oracle.CloudProviderName
	default:
		return ""
	}
}

// DetectCloudProvider detects the cloud provider where the agent is running in order. It first
// tries DetectCloudProviderDMI, which is network-free and therefore much faster; only when DMI
// can't tell us the cloud provider do we fall back to the network-based detectors.
func DetectCloudProvider(ctx context.Context, collectAccountID bool) (string, string) {
	if name := DetectCloudProviderDMI(); name != "" {
		log.Infof("Cloud provider %s detected via DMI", name)
		return name, detectCloudProviderAccountID(ctx, name, collectAccountID, cloudProviderDetectors[name].accountIDCallback)
	}

	for _, name := range cloudProviderDetectorResolutionOrder {
		cloudDetector := cloudProviderDetectors[name]
		if cloudDetector.callback(ctx) {
			log.Infof("Cloud provider %s detected", cloudDetector.name)
			return cloudDetector.name, detectCloudProviderAccountID(ctx, cloudDetector.name, collectAccountID, cloudDetector.accountIDCallback)
		}
	}
	log.Info("No cloud provider detected")
	return "", ""
}

// detectCloudProviderAccountID fetches the account ID for the given cloud provider, if requested
// and supported.
func detectCloudProviderAccountID(ctx context.Context, name string, collectAccountID bool, accountIDCallback func(context.Context) (string, error)) string {
	if !collectAccountID || accountIDCallback == nil {
		return ""
	}
	accountID, err := accountIDCallback(ctx)
	if err != nil {
		log.Debugf("Could not detect cloud provider account ID: %v", err)
		return ""
	}
	if accountID != "" {
		log.Infof("Detecting cloud provider account ID from %s: %+q", name, accountID)
	}
	return accountID
}

type cloudProviderNTPDetector struct {
	name     string
	callback func(context.Context) []string
}

var cloudProviderNTPDetectors = map[string]cloudProviderNTPDetector{
	ec2.CloudProviderName:     {name: ec2.CloudProviderName, callback: ec2.GetNTPHosts},
	gce.CloudProviderName:     {name: gce.CloudProviderName, callback: gce.GetNTPHosts},
	azure.CloudProviderName:   {name: azure.CloudProviderName, callback: azure.GetNTPHosts},
	alibaba.CloudProviderName: {name: alibaba.CloudProviderName, callback: alibaba.GetNTPHosts},
	tencent.CloudProviderName: {name: tencent.CloudProviderName, callback: tencent.GetNTPHosts},
	oracle.CloudProviderName:  {name: oracle.CloudProviderName, callback: oracle.GetNTPHosts},
}

var cloudProviderNTPDetectorResolutionOrder = []string{
	ec2.CloudProviderName,
	gce.CloudProviderName,
	azure.CloudProviderName,
	alibaba.CloudProviderName,
	tencent.CloudProviderName,
	oracle.CloudProviderName,
}

// GetCloudProviderNTPHosts detects the cloud provider where the agent is running and returns its
// NTP host name. If the cloud provider can be positively identified from DMI information, only
// that provider's NTP detector is queried, to avoid wasting calls on endpoints known not to apply
// to this host; the other providers are only probed as a fallback if that direct attempt fails.
func GetCloudProviderNTPHosts(ctx context.Context) []string {
	if provider := DetectCloudProviderDMI(); provider != "" {
		if detector, ok := cloudProviderNTPDetectors[provider]; ok {
			if cloudNTPServers := detector.callback(ctx); cloudNTPServers != nil {
				log.Infof("Detected %s cloud provider environment with NTP server(s) at %+q", detector.name, cloudNTPServers)
				return cloudNTPServers
			}
			log.Debugf("GetCloudProviderNTPHosts: could not retrieve NTP hosts from DMI-detected provider %s, falling back to probing all providers", provider)
		}
	}

	for _, name := range cloudProviderNTPDetectorResolutionOrder {
		cloudNTPDetector := cloudProviderNTPDetectors[name]
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
	// dmiDetectable marks cloud metadata detectors whose provider can be
	// positively identified via DMI. Once DMI identifies a provider, detectors
	// with dmiDetectable set for any other provider are skipped; all other
	// detectors (config, cloudfoundry, kubelet, kubernetes) always run
	// alongside it, since they either aren't cloud environments or can't be
	// distinguished from DMI information alone.
	dmiDetectable bool
	// requiresKubelet marks detectors that route through the shared kubelet
	// client singleton. Cluster Checks Runners are Deployment replicas, not
	// DaemonSets, so they are never colocated with a node's kubelet and can
	// never reach it; these detectors are skipped entirely on CCRs.
	requiresKubelet bool
	callback        func(context.Context) ([]string, error)
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

var hostAliasesDetectors = map[string]cloudProviderAliasesDetector{
	"config":                       {name: "config", callback: getValidHostAliases},
	alibaba.CloudProviderName:      {name: alibaba.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: alibaba.GetHostAliases},
	ec2.CloudProviderName:          {name: ec2.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: ec2.GetHostAliases},
	azure.CloudProviderName:        {name: azure.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: azure.GetHostAliases},
	gce.CloudProviderName:          {name: gce.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: gce.GetHostAliases},
	cloudfoundry.CloudProviderName: {name: cloudfoundry.CloudProviderName, isCloudEnv: true, callback: cloudfoundry.GetHostAliases},
	"kubelet":                      {name: "kubelet", requiresKubelet: true, callback: kubelet.GetHostAliases},
	tencent.CloudProviderName:      {name: tencent.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: tencent.GetHostAliases},
	oracle.CloudProviderName:       {name: oracle.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: oracle.GetHostAliases},
	ibm.CloudProviderName:          {name: ibm.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: ibm.GetHostAliases},
	kubernetes.CloudProviderName:   {name: kubernetes.CloudProviderName, requiresKubelet: true, callback: kubernetes.GetHostAliases},
}

var (
	hostAliasMutex   = sync.Mutex{}
	hostAliasLogOnce = true
)

// runHostAliasesDetectors runs the given host alias detectors concurrently and aggregates their
// results. Cloud providers endpoints can take a few seconds to answer, so we're using a WaitGroup
// to call all of them concurrently since GetHostAliases is called during the agent startup and is
// blocking.
func runHostAliasesDetectors(ctx context.Context, detectors map[string]cloudProviderAliasesDetector, isCLCRunner bool) ([]string, string) {
	aliases := []string{}
	cloudprovider := ""

	var wg sync.WaitGroup

	for _, hostAliasesDetector := range detectors {
		if isCLCRunner && hostAliasesDetector.requiresKubelet {
			// Skip probing the kubelet client singleton: it can never succeed on a
			// CCR and would otherwise trigger its exponential-backoff retrier and
			// its "Impossible to reach Kubelet" warning for no benefit.
			log.Debugf("Skipping %s Host Alias: Agent is a Cluster Checks Runner and has no reachable local Kubelet", hostAliasesDetector.name)
			continue
		}

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

// GetHostAliases returns the hostname aliases and the name of the possible cloud providers. If the
// cloud provider can be positively identified from DMI information, only that provider's alias
// detector is probed alongside the ones that aren't DMI-detectable (config, cloudfoundry, kubelet,
// kubernetes), so we're not stuck waiting on some irrelevant cloud provider's slow negative-case
// timeout; the full set of detectors is only probed as a fallback if that direct attempt doesn't
// yield a cloud provider.
func GetHostAliases(ctx context.Context) ([]string, string) {
	isCLCRunner := helper.IsCLCRunner(configsetup.Datadog())

	if provider := DetectCloudProviderDMI(); provider != "" {
		scoped := make(map[string]cloudProviderAliasesDetector, len(hostAliasesDetectors))
		for name, detector := range hostAliasesDetectors {
			if !detector.dmiDetectable || detector.name == provider {
				scoped[name] = detector
			}
		}

		aliases, cloudprovider := runHostAliasesDetectors(ctx, scoped, isCLCRunner)
		if cloudprovider != "" {
			log.Debugf("GetHostAliases: could not retrieve host aliases from DMI-detected provider %s, falling back to probing all providers", provider)
			return aliases, cloudprovider
		}
		return aliases, cloudprovider
	}

	return runHostAliasesDetectors(ctx, hostAliasesDetectors, isCLCRunner)
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

var publicIPv4Providers = map[string]func(context.Context) (string, error){
	ec2.CloudProviderName:   ec2.GetPublicIPv4,
	gce.CloudProviderName:   gce.GetPublicIPv4,
	azure.CloudProviderName: azure.GetPublicIPv4,
}

// GetPublicIPv4 returns the public IPv4 from different providers. If the cloud provider can be
// positively identified from DMI information, only that provider's metadata endpoint is queried
// first, to avoid wasting calls on endpoints known not to apply to this host; the other providers
// are only probed as a fallback if that direct attempt fails.
func GetPublicIPv4(ctx context.Context) (string, error) {
	if provider := DetectCloudProviderDMI(); provider != "" {
		if fetcher, ok := publicIPv4Providers[provider]; ok {
			publicIPv4, err := fetcher(ctx)
			if err == nil {
				log.Debugf("%s public IP: %s", provider, publicIPv4)
				return publicIPv4, nil
			}
			log.Debugf("Could not fetch %s public IPv4: %s, falling back to probing all providers", provider, err)
			return "", errors.New("No public IPv4 address found")
		}
	}

	for name, fetcher := range publicIPv4Providers {
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

var rebalanceDetectors = map[string]cloudProviderPreemptionDetector{
	ec2.CloudProviderName: ec2.GetRebalanceRecommendationTime,
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

// GetRebalanceRecommendationTime returns the time a rebalance recommendation was issued
// for a preemptible instance (e.g., AWS Spot rebalance recommendation).
// Returns ErrNotPreemptible if the instance is not preemptible.
// Returns ErrPreemptionUnsupported if the cloud provider doesn't support rebalance detection.
func GetRebalanceRecommendationTime(ctx context.Context, cloudProviderName string) (time.Time, error) {
	callback, found := rebalanceDetectors[cloudProviderName]
	if !found {
		return time.Time{}, ErrPreemptionUnsupported
	}

	noticeTime, err := callback(ctx)
	if err != nil {
		if errors.Is(err, ec2.ErrNotSpotInstance) {
			return time.Time{}, ErrNotPreemptible
		}
		return time.Time{}, err
	}
	return noticeTime, nil
}
