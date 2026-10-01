// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package cloudproviders

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/dmi"
)

// Mock setup mocks for the function 'DetectCloudProvider', 'GetSource' and 'GetHostID'
func Mock(t *testing.T, cloudProviderName string, accountIDCallback string, source string, hostID string) {
	// DetectCloudProvider checks DMI before consulting the detector maps below. Neutralize the
	// EC2/GCE/Azure DMI signals here so the mocked provider is actually used, regardless of the
	// real board vendor/product UUID/product name/chassis tag on the machine running the test
	// (e.g. a CI runner that is itself an EC2 instance); dmi.SetupMock* restores the original
	// values via t.Cleanup. Callers that need specific DMI values for their own fixtures (e.g.
	// display fields in a payload) must call dmi.SetupMock*/SetupMockProductName/
	// SetupMockChassisAssetTag again after Mock(), so their values win.
	dmi.SetupMock(t, "", "", "", "")
	dmi.SetupMockProductName(t, "")
	dmi.SetupMockChassisAssetTag(t, "")

	origDetectors := cloudProviderDetectors
	origResolutionOrder := cloudProviderDetectorResolutionOrder
	origGetSource := sourceDetectors
	orighostIDDetectors := hostIDDetectors
	origHostCCRIDDecectors := hostCCRIDDetectors
	origInstanceTypeDetectors := hostInstanceTypeDetectors

	t.Cleanup(func() {
		cloudProviderDetectors = origDetectors
		cloudProviderDetectorResolutionOrder = origResolutionOrder
		sourceDetectors = origGetSource
		hostIDDetectors = orighostIDDetectors
		hostCCRIDDetectors = origHostCCRIDDecectors
		hostInstanceTypeDetectors = origInstanceTypeDetectors
	})

	cloudProviderDetectors = map[string]cloudProviderDetector{
		cloudProviderName: {
			name:              cloudProviderName,
			callback:          func(context.Context) bool { return true },
			accountIDCallback: func(context.Context) (string, error) { return accountIDCallback, nil },
		},
	}
	cloudProviderDetectorResolutionOrder = []string{cloudProviderName}
	sourceDetectors = map[string]func() string{
		cloudProviderName: func() string { return source },
	}
	hostIDDetectors = map[string]func(context.Context) string{
		cloudProviderName: func(context.Context) string { return hostID },
	}
	hostCCRIDDetectors = map[string]cloudProviderCCRIDDetector{
		cloudProviderName: func(context.Context) (string, error) { return "test_ccrid", nil },
	}
	hostInstanceTypeDetectors = map[string]cloudProviderInstanceTypeDetector{
		cloudProviderName: func(context.Context) (string, error) { return "m5.medium", nil },
	}
}
