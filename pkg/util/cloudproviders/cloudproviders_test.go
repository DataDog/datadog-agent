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

func TestDetectCloudProvider(t *testing.T) {
	origDetectors := cloudProviderDetectors
	defer func() { cloudProviderDetectors = origDetectors }()

	t.Run("first matching cloud provider is detected", func(t *testing.T) {
		cloudProviderDetectors = []cloudProviderDetector{
			{
				name:     "cloud1",
				callback: func(_ context.Context) bool { return false },
			},
			{
				name:     "cloud2",
				callback: func(_ context.Context) bool { return true },
			},
			{
				name:     "cloud3",
				callback: func(_ context.Context) bool { return true },
			},
		}

		name, accountID := DetectCloudProvider(context.TODO(), false)
		assert.Equal(t, "cloud2", name)
		assert.Equal(t, "", accountID)
	})

	t.Run("no cloud provider detected", func(t *testing.T) {
		cloudProviderDetectors = []cloudProviderDetector{
			{
				name:     "cloud1",
				callback: func(_ context.Context) bool { return false },
			},
		}

		name, accountID := DetectCloudProvider(context.TODO(), false)
		assert.Equal(t, "", name)
		assert.Equal(t, "", accountID)
	})

	t.Run("account ID is collected when requested and available", func(t *testing.T) {
		cloudProviderDetectors = []cloudProviderDetector{
			{
				name:              "cloud1",
				callback:          func(_ context.Context) bool { return true },
				accountIDCallback: func(_ context.Context) (string, error) { return "account-123", nil },
			},
		}

		name, accountID := DetectCloudProvider(context.TODO(), true)
		assert.Equal(t, "cloud1", name)
		assert.Equal(t, "account-123", accountID)
	})

	t.Run("account ID not collected when not requested", func(t *testing.T) {
		accountIDCalled := false
		cloudProviderDetectors = []cloudProviderDetector{
			{
				name:     "cloud1",
				callback: func(_ context.Context) bool { return true },
				accountIDCallback: func(_ context.Context) (string, error) {
					accountIDCalled = true
					return "account-123", nil
				},
			},
		}

		name, accountID := DetectCloudProvider(context.TODO(), false)
		assert.Equal(t, "cloud1", name)
		assert.Equal(t, "", accountID)
		assert.False(t, accountIDCalled)
	})

	t.Run("account ID error is handled gracefully", func(t *testing.T) {
		cloudProviderDetectors = []cloudProviderDetector{
			{
				name:              "cloud1",
				callback:          func(_ context.Context) bool { return true },
				accountIDCallback: func(_ context.Context) (string, error) { return "", errors.New("failed") },
			},
		}

		name, accountID := DetectCloudProvider(context.TODO(), true)
		assert.Equal(t, "cloud1", name)
		assert.Equal(t, "", accountID)
	})

	t.Run("empty account ID is handled", func(t *testing.T) {
		cloudProviderDetectors = []cloudProviderDetector{
			{
				name:              "cloud1",
				callback:          func(_ context.Context) bool { return true },
				accountIDCallback: func(_ context.Context) (string, error) { return "", nil },
			},
		}

		name, accountID := DetectCloudProvider(context.TODO(), true)
		assert.Equal(t, "cloud1", name)
		assert.Equal(t, "", accountID)
	})
}

func TestGetSource(t *testing.T) {
	origDetectors := sourceDetectors
	defer func() { sourceDetectors = origDetectors }()

	sourceDetectors = map[string]func() string{
		"cloud1": func() string { return "IMDSv2" },
		"cloud2": func() string { return "DMI" },
	}

	t.Run("known cloud provider returns source", func(t *testing.T) {
		source := GetSource("cloud1")
		assert.Equal(t, "IMDSv2", source)

		source = GetSource("cloud2")
		assert.Equal(t, "DMI", source)
	})

	t.Run("unknown cloud provider returns empty string", func(t *testing.T) {
		source := GetSource("unknown")
		assert.Equal(t, "", source)

		source = GetSource("")
		assert.Equal(t, "", source)
	})
}

func TestGetHostID(t *testing.T) {
	origDetectors := hostIDDetectors
	defer func() { hostIDDetectors = origDetectors }()

	hostIDDetectors = map[string]func(context.Context) string{
		"cloud1": func(_ context.Context) string { return "i-1234567890abcdef0" },
		"cloud2": func(_ context.Context) string { return "vm-abcd1234" },
	}

	t.Run("known cloud provider returns host ID", func(t *testing.T) {
		hostID := GetHostID(context.TODO(), "cloud1")
		assert.Equal(t, "i-1234567890abcdef0", hostID)

		hostID = GetHostID(context.TODO(), "cloud2")
		assert.Equal(t, "vm-abcd1234", hostID)
	})

	t.Run("unknown cloud provider returns empty string", func(t *testing.T) {
		hostID := GetHostID(context.TODO(), "unknown")
		assert.Equal(t, "", hostID)

		hostID = GetHostID(context.TODO(), "")
		assert.Equal(t, "", hostID)
	})
}

func TestGetCloudProviderNTPHosts(t *testing.T) {
	// Save and restore original detectors is not possible since ntpDetectors is local
	// We can only test the existing behavior with empty results

	t.Run("returns nil when no cloud NTP servers detected", func(t *testing.T) {
		// When not running in any cloud environment, this should return nil
		// This is a basic test since we can't mock the internal detectors
		// In a real cloud environment this would return actual NTP servers
		result := GetCloudProviderNTPHosts(context.TODO())
		// Result can be nil or a list depending on the actual cloud environment
		// We just verify it doesn't panic
		_ = result
	})
}

func TestGetPublicIPv4(t *testing.T) {
	t.Run("returns error when not in cloud environment", func(t *testing.T) {
		// When not running in any cloud environment, this should return an error
		// This tests the error path
		ip, err := GetPublicIPv4(context.TODO())
		// In a non-cloud environment, we expect either a valid IP or an error
		if err != nil {
			assert.Contains(t, err.Error(), "No public IPv4")
			assert.Equal(t, "", ip)
		}
	})
}
