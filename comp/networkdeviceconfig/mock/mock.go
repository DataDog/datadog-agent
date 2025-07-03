// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the networkdeviceconfig component
package mock

import (
	networkdeviceconfig "github.com/DataDog/datadog-agent/comp/networkdeviceconfig/def"
)

type networkDeviceConfigMock struct{}

func newMock() networkdeviceconfig.Component {
	return &networkDeviceConfigMock{}
}

func (n *networkDeviceConfigMock) RetrieveConfiguration(_ string) (string, error) {
	return "Building configuration...\nCurrent configuration : 3781 bytes\n!\nversion 12.3", nil
}
