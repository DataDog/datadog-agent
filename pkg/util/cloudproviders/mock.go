// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package cloudproviders

import (
	"context"
	"testing"
)

// Mock setup mocks for the function 'DetectCloudProvider', 'GetSource' and 'GetHostID'
func Mock(t *testing.T, cloudProviderName string, accountIDCallback string, source string, hostID string) {
	origDetectors := cloudProviderDetectors
	origGetSource := sourceDetectors
	orighostIDDetectors := hostIDDetectors

	t.Cleanup(func() {
		cloudProviderDetectors = origDetectors
		sourceDetectors = origGetSource
		hostIDDetectors = orighostIDDetectors
	})

	cloudProviderDetectors = []cloudProviderDetector{
		{
			name:              cloudProviderName,
			callback:          func(context.Context) bool { return true },
			accountIDCallback: func(context.Context) (string, error) { return accountIDCallback, nil },
		},
	}
	sourceDetectors = map[string]func() string{
		cloudProviderName: func() string { return source },
	}
	hostIDDetectors = map[string]func(context.Context) string{
		cloudProviderName: func(context.Context) string { return hostID },
	}
}
