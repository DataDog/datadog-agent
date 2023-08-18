// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package hostname

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
)

func TestHostnameCaching(t *testing.T) {
}

// testCase represents a test scenario for hostname resolution. The logic goes down a list trying different provider
// that might or might not be coupled. Each field represents if the corresponding provider should be successful or not
// and which one we expect at the end.
type testCase struct {
	name             string
	configHostname   bool
	hostnameFile     bool
	fargate          bool
	GCE              bool
	azure            bool
	container        bool
	FQDN             bool
	FQDNEC2          bool
	OS               bool
	OSEC2            bool
	EC2              bool
	EC2Proritized    bool
	expectedHostname string
	expectedProvider string
}

func setupHostnameTest(t *testing.T, tc testCase) {
	t.Cleanup(func() {
		isFargateInstance = fargate.IsFargateInstance
		ec2GetInstanceID = ec2.GetInstanceID
		isContainerized = config.IsContainerized
		gceGetHostname = gce.GetHostname
		azureGetHostname = azure.GetHostname
		osHostname = os.Hostname
		fqdnHostname = getSystemFQDN
		osHostnameUsable = isOSHostnameUsable

		// erase cache
		cache.Cache.Delete(cache.BuildAgentKey("hostname"))
	})
	config.Mock(t)

	if tc.configHostname {
		config.Datadog.Set("hostname", "hostname-from-configuration")
	}
	if tc.hostnameFile {
		setupHostnameFile(t, "hostname-from-file")
	}
	if tc.fargate {
		isFargateInstance = func() bool { return true }
	} else {
		isFargateInstance = func() bool { return false }
	}

	if tc.GCE {
		gceGetHostname = func(context.Context) (string, error) { return "hostname-from-gce", nil }
	} else {
		gceGetHostname = func(context.Context) (string, error) { return "", fmt.Errorf("some error") }
	}

	if tc.azure {
		azureGetHostname = func(context.Context) (string, error) { return "hostname-from-azure", nil }
	} else {
		azureGetHostname = func(context.Context) (string, error) { return "", fmt.Errorf("some error") }
	}

	if tc.FQDN || tc.FQDNEC2 {
		// making isOSHostnameUsable return true
		osHostnameUsable = func(ctx context.Context) bool { return true }
		config.Datadog.Set("hostname_fqdn", true)
		if !tc.FQDNEC2 {
			fqdnHostname = func() (string, error) { return "hostname-from-fqdn", nil }
		} else {
			fqdnHostname = func() (string, error) { return "ip-default-ec2-hostname", nil }
		}
	} else {
		fqdnHostname = func() (string, error) { return "", fmt.Errorf("some error") }
	}

	if tc.OS || tc.OSEC2 {
		// making isOSHostnameUsable return true
		osHostnameUsable = func(ctx context.Context) bool { return true }
		if !tc.OSEC2 {
			osHostname = func() (string, error) { return "hostname-from-os", nil }
		} else {
			osHostname = func() (string, error) { return "ip-default-ec2-hostname", nil }
		}
	} else {
		osHostname = func() (string, error) { return "", fmt.Errorf("some error") }
	}

	if tc.EC2 {
		ec2GetInstanceID = func(context.Context) (string, error) { return "hostname-from-ec2", nil }
	} else {
		ec2GetInstanceID = func(context.Context) (string, error) { return "", fmt.Errorf("some error") }
	}

	if tc.EC2Proritized {
		config.Datadog.Set("ec2_prioritize_instance_id_as_hostname", true)
	}
}

func TestFromConfigurationFalse(t *testing.T) {
	setupHostnameTest(t, testCase{
		name:             "configuration hostname file",
		configHostname:   false,
		hostnameFile:     true,
		fargate:          true,
		GCE:              true,
		azure:            true,
		container:        true,
		FQDN:             true,
		OS:               true,
		EC2:              true,
		EC2Proritized:    true,
		expectedHostname: "hostname-from-file",
		expectedProvider: "hostnameFile",
	})
	data, err := GetWithProvider(context.TODO())
	assert.NoError(t, err)
	assert.False(t, data.FromConfiguration())
}

func TestFromConfigurationTrue(t *testing.T) {
	setupHostnameTest(t, testCase{
		name:             "configuration hostname",
		configHostname:   true,
		hostnameFile:     true,
		fargate:          true,
		GCE:              true,
		azure:            true,
		container:        true,
		FQDN:             true,
		OS:               true,
		EC2:              true,
		EC2Proritized:    true,
		expectedHostname: "hostname-from-configuration",
		expectedProvider: configProvider,
	})

	data, err := GetWithProvider(context.TODO())
	assert.NoError(t, err)
	assert.True(t, data.FromConfiguration())
}

func TestHostnamePrority(t *testing.T) {
	hostnameTestCases := []testCase{
		{
			name:             "configuration hostname",
			configHostname:   true,
			hostnameFile:     true,
			fargate:          true,
			GCE:              true,
			azure:            true,
			container:        true,
			FQDN:             true,
			OS:               true,
			EC2:              true,
			EC2Proritized:    true,
			expectedHostname: "hostname-from-configuration",
			expectedProvider: configProvider,
		},
		{
			name:             "configuration hostname file",
			configHostname:   false,
			hostnameFile:     true,
			fargate:          true,
			GCE:              true,
			azure:            true,
			container:        true,
			FQDN:             true,
			OS:               true,
			EC2:              true,
			EC2Proritized:    true,
			expectedHostname: "hostname-from-file",
			expectedProvider: "hostnameFile",
		},
		{
			name:             "hostname on fargate",
			configHostname:   false,
			hostnameFile:     false,
			fargate:          true,
			GCE:              true,
			azure:            true,
			container:        true,
			FQDN:             true,
			OS:               true,
			EC2:              true,
			EC2Proritized:    true,
			expectedHostname: "",
			expectedProvider: "fargate",
		},
		{
			name:             "hostname from GCE",
			configHostname:   false,
			hostnameFile:     false,
			fargate:          false,
			GCE:              true,
			azure:            true,
			container:        true,
			FQDN:             true,
			OS:               true,
			EC2:              true,
			EC2Proritized:    true,
			expectedHostname: "hostname-from-gce",
			expectedProvider: "gce",
		},
		{
			name:             "hostname from Azure",
			configHostname:   false,
			hostnameFile:     false,
			fargate:          false,
			GCE:              false,
			azure:            true,
			container:        true,
			FQDN:             true,
			OS:               true,
			EC2:              true,
			EC2Proritized:    true,
			expectedHostname: "hostname-from-azure",
			expectedProvider: "azure",
		},
		{
			name:             "hostname from FQDN",
			configHostname:   false,
			hostnameFile:     false,
			fargate:          false,
			GCE:              false,
			azure:            false,
			container:        false,
			FQDN:             true,
			OS:               true,
			EC2:              true,
			EC2Proritized:    false, // no prority to EC2
			expectedHostname: "hostname-from-fqdn",
			expectedProvider: "fqdn",
		},
		{
			name:             "hostname from OS",
			configHostname:   false,
			hostnameFile:     false,
			fargate:          false,
			GCE:              false,
			azure:            false,
			container:        false,
			FQDN:             false,
			OS:               true,
			EC2:              true,
			EC2Proritized:    false, // no prority to EC2
			expectedHostname: "hostname-from-os",
			expectedProvider: "os",
		},
		{
			name:             "hostname from EC2 prioritized",
			configHostname:   false,
			hostnameFile:     false,
			fargate:          false,
			GCE:              false,
			azure:            false,
			container:        false,
			FQDN:             true,
			OS:               true,
			EC2:              true,
			EC2Proritized:    true,
			expectedHostname: "hostname-from-ec2",
			expectedProvider: "aws",
		},
		{
			name:             "hostname from EC2 prioritized",
			configHostname:   false,
			hostnameFile:     false,
			fargate:          false,
			GCE:              false,
			azure:            false,
			container:        false,
			FQDN:             true,
			OS:               true,
			EC2:              true,
			EC2Proritized:    true,
			expectedHostname: "hostname-from-ec2",
			expectedProvider: "aws",
		},
		{
			// When os/fqdn hostname is the default hostname from an EC2 instance we want to fallback on the
			// instance ID
			name:             "hostname from EC2 with default system name",
			configHostname:   false,
			hostnameFile:     false,
			fargate:          false,
			GCE:              false,
			azure:            false,
			container:        false,
			FQDNEC2:          true, // using the EC2 flavor
			OSEC2:            true, // using the EC2 flavor
			EC2:              true,
			EC2Proritized:    false, // no prority to EC2. We want to naturally fallback on it
			expectedHostname: "hostname-from-ec2",
			expectedProvider: "aws",
		},
		{
			name:             "No hostname detected",
			configHostname:   false,
			hostnameFile:     false,
			fargate:          false,
			GCE:              false,
			azure:            false,
			container:        false,
			FQDNEC2:          false,
			OSEC2:            false,
			EC2:              false,
			EC2Proritized:    false,
			expectedHostname: "",
			expectedProvider: "",
		},
	}

	for _, tc := range hostnameTestCases {
		t.Run(tc.name, func(t *testing.T) {
			setupHostnameTest(t, tc)

			data, err := GetWithProvider(context.TODO())
			if tc.expectedProvider == "" {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedHostname, data.Hostname)
				assert.Equal(t, tc.expectedProvider, data.Provider)

				// check cache
				cacheHostname, found := cache.Cache.Get(cache.BuildAgentKey("hostname"))
				assert.True(t, found, "hostname data was not cached")
				assert.Equal(t, data, cacheHostname)
			}
		})
	}
}
