// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudproviders

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/oracle"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectCloudProviderDMI(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)
	cfg.SetInTest("gce_use_dmi", true)
	cfg.SetInTest("azure_use_dmi", true)
	cfg.SetInTest("oracle_use_dmi", true)

	dmi.SetupMock(t, "", "", "", "")
	dmi.SetupMockProductName(t, "")
	dmi.SetupMockChassisAssetTag(t, "")
	assert.Equal(t, "", DetectCloudProviderDMI())

	dmi.SetupMockProductName(t, gce.DMIProductName)
	assert.Equal(t, gce.CloudProviderName, DetectCloudProviderDMI())
	dmi.SetupMockProductName(t, "")

	dmi.SetupMockChassisAssetTag(t, azure.DMIChassisAssetTag)
	assert.Equal(t, azure.CloudProviderName, DetectCloudProviderDMI())
	dmi.SetupMockChassisAssetTag(t, "")

	dmi.SetupMockChassisAssetTag(t, oracle.DMIChassisAssetTag)
	assert.Equal(t, oracle.CloudProviderName, DetectCloudProviderDMI())
	dmi.SetupMockChassisAssetTag(t, "")

	dmi.SetupMock(t, "", "", "i-myinstance", ec2.DMIBoardVendor)
	assert.Equal(t, ec2.CloudProviderName, DetectCloudProviderDMI())
}

func TestDetectCloudProviderShortCircuitsNetworkCallsWhenDMIMatches(t *testing.T) {
	origDetectors := cloudProviderDetectors
	origResolutionOrder := cloudProviderDetectorResolutionOrder
	defer func() {
		cloudProviderDetectors = origDetectors
		cloudProviderDetectorResolutionOrder = origResolutionOrder
	}()

	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)
	cfg.SetInTest("gce_use_dmi", true)
	cfg.SetInTest("azure_use_dmi", true)
	dmi.SetupMock(t, "", "", "", "")
	dmi.SetupMockChassisAssetTag(t, "")
	dmi.SetupMockProductName(t, gce.DMIProductName)

	networkCallbackCalled := false
	cloudProviderDetectors = map[string]cloudProviderDetector{
		"network-detector": {name: "network-detector", callback: func(context.Context) bool {
			networkCallbackCalled = true
			return true
		}},
	}
	cloudProviderDetectorResolutionOrder = []string{"network-detector"}

	name, _ := DetectCloudProvider(context.TODO(), false)
	assert.Equal(t, gce.CloudProviderName, name)
	assert.False(t, networkCallbackCalled, "network callback should not be called when DMI already detected the cloud provider")
}

func TestDetectCloudProviderFallsBackToNetworkWhenDMIInconclusive(t *testing.T) {
	origDetectors := cloudProviderDetectors
	origResolutionOrder := cloudProviderDetectorResolutionOrder
	defer func() {
		cloudProviderDetectors = origDetectors
		cloudProviderDetectorResolutionOrder = origResolutionOrder
	}()

	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)
	cfg.SetInTest("gce_use_dmi", true)
	cfg.SetInTest("azure_use_dmi", true)
	dmi.SetupMock(t, "", "", "", "")
	dmi.SetupMockProductName(t, "")
	dmi.SetupMockChassisAssetTag(t, "")

	cloudProviderDetectors = map[string]cloudProviderDetector{
		"network-detector": {name: "network-detector", callback: func(context.Context) bool { return true }},
	}
	cloudProviderDetectorResolutionOrder = []string{"network-detector"}

	name, _ := DetectCloudProvider(context.TODO(), false)
	assert.Equal(t, "network-detector", name)
}

func TestCloudProviderAliases(t *testing.T) {
	origDetectors := hostAliasesDetectors
	defer func() { hostAliasesDetectors = origDetectors }()

	detector1Called := false
	detector2Called := false
	detector3Called := false

	hostAliasesDetectors = map[string]cloudProviderAliasesDetector{
		"detector1": {
			name:       "detector1",
			isCloudEnv: true,
			callback: func(_ context.Context) ([]string, error) {
				detector1Called = true
				return []string{"alias2"}, nil
			},
		},
		"detector2": {
			name:       "detector2",
			isCloudEnv: true,
			callback: func(_ context.Context) ([]string, error) {
				detector2Called = true
				return nil, errors.New("error from detector2")
			},
		},
		"detector3": {
			name:       "detector3",
			isCloudEnv: true,
			callback: func(_ context.Context) ([]string, error) {
				detector3Called = true
				return []string{"alias1", "alias2"}, nil
			},
		},
	}

	aliases, cloudprovider := GetHostAliases(context.TODO())
	assert.True(t, detector1Called, "host alias callback for 'detector1' was not called")
	assert.True(t, detector2Called, "host alias callback for 'detector2' was not called")
	assert.True(t, detector3Called, "host alias callback for 'detector3' was not called")

	assert.Equal(t, []string{"alias1", "alias2"}, aliases)
	// Which detector wins depends upon timing, either one is fine
	// In reality we expect only 1 possible cloudprovider to return host aliases
	assert.Contains(t, []string{"detector1", "detector3"}, cloudprovider)
}

// TestCloudProviderAliasesSkipsKubeletDependentDetectorsOnCLCRunner ensures
// that GetHostAliases never invokes the "kubelet" or "kubernetes" detectors
// on a Cluster Checks Runner, since CCRs are Deployment replicas (not
// DaemonSets) and can never reach a local kubelet.
func TestCloudProviderAliasesSkipsKubeletDependentDetectorsOnCLCRunner(t *testing.T) {
	origDetectors := hostAliasesDetectors
	defer func() { hostAliasesDetectors = origDetectors }()

	config := configmock.New(t)
	config.SetInTest("clc_runner_enabled", true)
	config.SetInTest("config_providers", []map[string]interface{}{{"name": "clusterchecks"}})

	kubeletCalled := false
	kubernetesDetectorCalled := false
	otherDetectorCalled := false

	hostAliasesDetectors = map[string]cloudProviderAliasesDetector{
		"kubelet": {
			name:            "kubelet",
			requiresKubelet: true,
			callback: func(_ context.Context) ([]string, error) {
				kubeletCalled = true
				return []string{"kubelet-alias"}, nil
			},
		},
		"kubernetes": {
			name:            "kubernetes",
			requiresKubelet: true,
			callback: func(_ context.Context) ([]string, error) {
				kubernetesDetectorCalled = true
				return []string{"kubernetes-alias"}, nil
			},
		},
		"other": {
			name:       "other",
			isCloudEnv: true,
			callback: func(_ context.Context) ([]string, error) {
				otherDetectorCalled = true
				return []string{"other-alias"}, nil
			},
		},
	}

	aliases, cloudprovider := GetHostAliases(context.TODO())
	assert.False(t, kubeletCalled, "kubelet host alias detector should be skipped on a Cluster Checks Runner")
	assert.False(t, kubernetesDetectorCalled, "kubernetes host alias detector should be skipped on a Cluster Checks Runner")
	assert.True(t, otherDetectorCalled, "non-kubelet-dependent host alias detectors should still run on a Cluster Checks Runner")
	assert.Equal(t, []string{"other-alias"}, aliases)
	assert.Equal(t, "other", cloudprovider)
}

// setupDMIProvider mocks DMI so that DetectCloudProviderDMI() returns the given provider (or ""
// for an inconclusive result, when provider is empty).
func setupDMIProvider(t *testing.T, provider string) {
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)
	cfg.SetInTest("gce_use_dmi", true)
	cfg.SetInTest("azure_use_dmi", true)

	dmi.SetupMock(t, "", "", "", "")
	dmi.SetupMockProductName(t, "")
	dmi.SetupMockChassisAssetTag(t, "")

	switch provider {
	case ec2.CloudProviderName:
		dmi.SetupMock(t, "", "", "i-myinstance", ec2.DMIBoardVendor)
	case gce.CloudProviderName:
		dmi.SetupMockProductName(t, gce.DMIProductName)
	case azure.CloudProviderName:
		dmi.SetupMockChassisAssetTag(t, azure.DMIChassisAssetTag)
	}
}

func TestGetHostAliasesUsesDMIDetectedProviderDirectly(t *testing.T) {
	origDetectors := hostAliasesDetectors
	defer func() { hostAliasesDetectors = origDetectors }()
	setupDMIProvider(t, ec2.CloudProviderName)

	configCalled, ec2Called, gceCalled := false, false, false
	hostAliasesDetectors = map[string]cloudProviderAliasesDetector{
		"config": {name: "config", callback: func(_ context.Context) ([]string, error) {
			configCalled = true
			return []string{"config-alias"}, nil
		}},
		ec2.CloudProviderName: {name: ec2.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: func(_ context.Context) ([]string, error) {
			ec2Called = true
			return []string{"ec2-alias"}, nil
		}},
		gce.CloudProviderName: {name: gce.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: func(_ context.Context) ([]string, error) {
			gceCalled = true
			return []string{"gce-alias"}, nil
		}},
	}

	aliases, cloudprovider := GetHostAliases(context.TODO())
	assert.True(t, configCalled, "non-cloud detectors should still run")
	assert.True(t, ec2Called, "the DMI-detected provider's detector should run")
	assert.False(t, gceCalled, "detectors for other cloud providers should not run once one is positively detected via DMI")
	assert.Equal(t, ec2.CloudProviderName, cloudprovider)
	assert.ElementsMatch(t, []string{"config-alias", "ec2-alias"}, aliases)
}

func TestGetHostAliasesFallsBackWhenDMIDetectedProviderFails(t *testing.T) {
	origDetectors := hostAliasesDetectors
	defer func() { hostAliasesDetectors = origDetectors }()
	setupDMIProvider(t, ec2.CloudProviderName)

	gceCalled := false
	hostAliasesDetectors = map[string]cloudProviderAliasesDetector{
		ec2.CloudProviderName: {name: ec2.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: func(_ context.Context) ([]string, error) {
			return nil, errors.New("ec2 metadata unreachable")
		}},
		gce.CloudProviderName: {name: gce.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: func(_ context.Context) ([]string, error) {
			gceCalled = true
			return []string{"gce-alias"}, nil
		}},
	}

	aliases, cloudprovider := GetHostAliases(context.TODO())
	assert.True(t, gceCalled, "should fall back to probing all providers when the DMI-detected provider's fetch fails")
	assert.Equal(t, gce.CloudProviderName, cloudprovider)
	assert.Equal(t, []string{"gce-alias"}, aliases)
}

// TestGetHostAliasesAlwaysRunsNonDMIDetectableDetectors ensures that detectors that aren't
// DMI-detectable (config, cloudfoundry, kubelet, kubernetes) still run alongside the
// DMI-detected provider's detector, since they either aren't cloud environments or can't be
// distinguished from DMI information alone.
func TestGetHostAliasesAlwaysRunsNonDMIDetectableDetectors(t *testing.T) {
	origDetectors := hostAliasesDetectors
	defer func() { hostAliasesDetectors = origDetectors }()
	setupDMIProvider(t, ec2.CloudProviderName)

	configCalled, cloudfoundryCalled, kubeletCalled, kubernetesCalled, ec2Called, gceCalled := false, false, false, false, false, false
	hostAliasesDetectors = map[string]cloudProviderAliasesDetector{
		"config": {name: "config", callback: func(_ context.Context) ([]string, error) {
			configCalled = true
			return nil, nil
		}},
		"cloudfoundry": {name: "cloudfoundry", isCloudEnv: true, callback: func(_ context.Context) ([]string, error) {
			cloudfoundryCalled = true
			return nil, nil
		}},
		"kubelet": {name: "kubelet", requiresKubelet: true, callback: func(_ context.Context) ([]string, error) {
			kubeletCalled = true
			return nil, nil
		}},
		"kubernetes": {name: "kubernetes", requiresKubelet: true, callback: func(_ context.Context) ([]string, error) {
			kubernetesCalled = true
			return nil, nil
		}},
		ec2.CloudProviderName: {name: ec2.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: func(_ context.Context) ([]string, error) {
			ec2Called = true
			return []string{"ec2-alias"}, nil
		}},
		gce.CloudProviderName: {name: gce.CloudProviderName, isCloudEnv: true, dmiDetectable: true, callback: func(_ context.Context) ([]string, error) {
			gceCalled = true
			return []string{"gce-alias"}, nil
		}},
	}

	aliases, cloudprovider := GetHostAliases(context.TODO())
	assert.True(t, configCalled, "config detector should always run")
	assert.True(t, cloudfoundryCalled, "cloudfoundry detector should always run, since it can't be detected via DMI")
	assert.True(t, kubeletCalled, "kubelet detector should always run")
	assert.True(t, kubernetesCalled, "kubernetes detector should always run")
	assert.True(t, ec2Called, "the DMI-detected provider's detector should run")
	assert.False(t, gceCalled, "detectors for other DMI-detectable providers should not run once one is positively detected via DMI")
	assert.Equal(t, ec2.CloudProviderName, cloudprovider)
	assert.Equal(t, []string{"ec2-alias"}, aliases)
}

func TestGetPublicIPv4UsesDMIDetectedProviderDirectly(t *testing.T) {
	origProviders := publicIPv4Providers
	defer func() { publicIPv4Providers = origProviders }()
	setupDMIProvider(t, gce.CloudProviderName)

	ec2Called, gceCalled := false, false
	publicIPv4Providers = map[string]func(context.Context) (string, error){
		ec2.CloudProviderName: func(_ context.Context) (string, error) {
			ec2Called = true
			return "", errors.New("should not be called")
		},
		gce.CloudProviderName: func(_ context.Context) (string, error) {
			gceCalled = true
			return "1.2.3.4", nil
		},
	}

	ip, err := GetPublicIPv4(context.TODO())
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.4", ip)
	assert.True(t, gceCalled, "the DMI-detected provider should be probed directly")
	assert.False(t, ec2Called, "other providers should not be probed once one is positively detected via DMI")
}

func TestGetPublicIPv4FallsBackWhenDMIDetectedProviderFails(t *testing.T) {
	origProviders := publicIPv4Providers
	defer func() { publicIPv4Providers = origProviders }()
	setupDMIProvider(t, gce.CloudProviderName)

	ec2Called, gceCalled := false, 0
	publicIPv4Providers = map[string]func(context.Context) (string, error){
		ec2.CloudProviderName: func(_ context.Context) (string, error) {
			ec2Called = true
			return "5.6.7.8", nil
		},
		gce.CloudProviderName: func(_ context.Context) (string, error) {
			gceCalled++
			return "", errors.New("gce metadata unreachable")
		},
	}

	ip, err := GetPublicIPv4(context.TODO())
	require.NoError(t, err)
	assert.Equal(t, "5.6.7.8", ip)
	assert.True(t, ec2Called, "should fall back to probing all providers when the DMI-detected provider's fetch fails")
	assert.GreaterOrEqual(t, gceCalled, 1, "the DMI-detected provider should have been probed directly at least once")
}

func TestCloudProviderHostCCRID(t *testing.T) {
	origDetectors := hostCCRIDDetectors
	defer func() { hostCCRIDDetectors = origDetectors }()

	detector1Called := false
	detector2Called := false
	clearDetectors := func() {
		detector1Called = false
		detector2Called = false
	}

	hostCCRIDDetectors = map[string]cloudProviderCCRIDDetector{
		"detector1": func(_ context.Context) (string, error) {
			detector1Called = true
			return "ccrid1", nil
		},
		"detector2": func(_ context.Context) (string, error) {
			detector2Called = true
			return "ccrid2", nil
		},
	}

	ccrid := GetHostCCRID(context.TODO(), "detector2")
	assert.False(t, detector1Called, "host alias callback for 'detector1' should not be called")
	assert.True(t, detector2Called, "host alias callback for 'detector2' was not called")
	assert.Equal(t, "ccrid2", ccrid)
	clearDetectors()

	ccrid = GetHostCCRID(context.TODO(), "detector1")
	assert.True(t, detector1Called, "host alias callback for 'detector1' was not called")
	assert.False(t, detector2Called, "host alias callback for 'detector2' should not be called")
	assert.Equal(t, "ccrid1", ccrid)
	clearDetectors()

	// If not a known environment, try everything
	ccrid = GetHostCCRID(context.TODO(), "kubelet")
	assert.True(t, detector1Called, "host alias callback for 'detector1' was not called")
	assert.True(t, detector2Called, "host alias callback for 'detector2' was not called")
	assert.True(t, strings.HasPrefix(ccrid, "ccrid"))
	clearDetectors()

	// If an empty string, fail fast
	ccrid = GetHostCCRID(context.TODO(), "")
	assert.False(t, detector1Called, "host alias callback for 'detector1' should not be called")
	assert.False(t, detector2Called, "host alias callback for 'detector2' should not be called")
	assert.Equal(t, "", ccrid)
	clearDetectors()
}

func TestGetValidHostAliasesWithConfig(t *testing.T) {
	config := configmock.New(t)
	config.SetInTest("host_aliases", []string{"foo", "-bar"})

	val, err := getValidHostAliases(context.TODO())
	require.NoError(t, err)
	assert.EqualValues(t, []string{"foo"}, val)
}

func TestCloudProviderInstanceType(t *testing.T) {
	origDetectors := hostInstanceTypeDetectors
	defer func() { hostInstanceTypeDetectors = origDetectors }()

	detector1Called := false
	detector2Called := false
	clearDetectors := func() {
		detector1Called = false
		detector2Called = false
	}

	hostInstanceTypeDetectors = map[string]cloudProviderInstanceTypeDetector{
		"detector1": func(_ context.Context) (string, error) {
			detector1Called = true
			return "t3.medium", nil
		},
		"detector2": func(_ context.Context) (string, error) {
			detector2Called = true
			return "m5.large", nil
		},
	}

	// Case 1: known cloud provider "detector2" should only call detector2
	instanceType := GetInstanceType(context.TODO(), "detector2")
	assert.False(t, detector1Called, "instance type callback for 'detector1' should not be called")
	assert.True(t, detector2Called, "instance type callback for 'detector2' was not called")
	assert.Equal(t, "m5.large", instanceType)
	clearDetectors()

	// Case 2: known cloud provider "detector1" should only call detector1
	instanceType = GetInstanceType(context.TODO(), "detector1")
	assert.True(t, detector1Called, "instance type callback for 'detector1' was not called")
	assert.False(t, detector2Called, "instance type callback for 'detector2' should not be called")
	assert.Equal(t, "t3.medium", instanceType)
	clearDetectors()

	// Case 3: unknown provider
	instanceType = GetInstanceType(context.TODO(), "kubelet")
	assert.Equal(t, "", instanceType)
	clearDetectors()

	// Case 4: empty detected cloud — should fail fast
	instanceType = GetInstanceType(context.TODO(), "")
	assert.False(t, detector1Called, "instance type callback for 'detector1' should not be called")
	assert.False(t, detector2Called, "instance type callback for 'detector2' should not be called")
	assert.Equal(t, "", instanceType)
	clearDetectors()
}

func TestCloudProviderPreemptionTerminationTime(t *testing.T) {
	origDetectors := preemptionDetectors
	defer func() { preemptionDetectors = origDetectors }()

	expectedTime := time.Now()

	detector1Called := false
	detector2Called := false
	clearDetectors := func() {
		detector1Called = false
		detector2Called = false
	}

	preemptionDetectors = map[string]cloudProviderPreemptionDetector{
		"detector1": func(_ context.Context) (time.Time, error) {
			detector1Called = true
			return expectedTime, nil
		},
		"detector2": func(_ context.Context) (time.Time, error) {
			detector2Called = true
			return time.Time{}, errors.New("no preemption scheduled")
		},
	}

	// Case 1: known cloud provider "detector1" with termination scheduled
	terminationTime, err := GetPreemptionTerminationTime(context.TODO(), "detector1")
	assert.True(t, detector1Called, "preemption callback for 'detector1' was not called")
	assert.False(t, detector2Called, "preemption callback for 'detector2' should not be called")
	require.NoError(t, err)
	assert.Equal(t, expectedTime, terminationTime)
	clearDetectors()

	// Case 2: known cloud provider "detector2" with no termination scheduled (returns error)
	terminationTime, err = GetPreemptionTerminationTime(context.TODO(), "detector2")
	assert.False(t, detector1Called, "preemption callback for 'detector1' should not be called")
	assert.True(t, detector2Called, "preemption callback for 'detector2' was not called")
	require.Error(t, err)
	assert.Equal(t, time.Time{}, terminationTime)
	clearDetectors()

	// Case 3: unknown provider should return error
	terminationTime, err = GetPreemptionTerminationTime(context.TODO(), "unknown")
	assert.False(t, detector1Called, "preemption callback for 'detector1' should not be called")
	assert.False(t, detector2Called, "preemption callback for 'detector2' should not be called")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
	assert.Equal(t, time.Time{}, terminationTime)
	clearDetectors()

	// Case 4: empty cloud provider should return error
	terminationTime, err = GetPreemptionTerminationTime(context.TODO(), "")
	assert.False(t, detector1Called, "preemption callback for 'detector1' should not be called")
	assert.False(t, detector2Called, "preemption callback for 'detector2' should not be called")
	require.Error(t, err)
	assert.Equal(t, time.Time{}, terminationTime)
	clearDetectors()
}
