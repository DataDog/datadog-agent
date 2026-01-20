// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package hostsysteminfoimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

// getTestHostSystemInfo creates a test hostsysteminfoimpl instance with mocked dependencies
func getTestHostSystemInfo(t *testing.T, overrides map[string]any) *hostSystemInfo {
	if overrides == nil {
		overrides = map[string]any{
			"infrastructure_mode": "end_user_device",
		}
	}

	// Use mock hostname service to avoid network timeouts
	hostname, _ := hostnameinterface.NewMock(hostnameinterface.MockHostname("test-hostname"))

	p := NewSystemInfoProvider(Requires{
		Log:        logmock.New(t),
		Config:     config.NewMockWithOverrides(t, overrides),
		Serializer: serializermock.NewMetricSerializer(t),
		Hostname:   hostname,
	})

	return p.Comp.(*hostSystemInfo)
}
