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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudProviderAliases(t *testing.T) {
	origDetectors := hostAliasesDetectors
	defer func() { hostAliasesDetectors = origDetectors }()

	detector1Called := false
	detector2Called := false
	detector3Called := false

	hostAliasesDetectors = []cloudProviderAliasesDetector{
		{
			name:       "detector1",
			isCloudEnv: true,
			callback: func(_ context.Context) ([]string, error) {
				detector1Called = true
				return []string{"alias2"}, nil
			},
		},
		{
			name:       "detector2",
			isCloudEnv: true,
			callback: func(_ context.Context) ([]string, error) {
				detector2Called = true
				return nil, errors.New("error from detector2")
			},
		},
		{
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
	config.SetWithoutSource("host_aliases", []string{"foo", "-bar"})

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
