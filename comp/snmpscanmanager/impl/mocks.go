// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"

	"github.com/stretchr/testify/mock"
)

type snmpConfigProviderMock struct {
	mock.Mock
}

func newSnmpConfigProviderMock() *snmpConfigProviderMock {
	return &snmpConfigProviderMock{}
}

func (m *snmpConfigProviderMock) GetDeviceConfig(deviceIP string, agentConfig config.Component, httpClient ipc.HTTPClient) (*snmpparse.SNMPConfig, string, error) {
	args := m.Called(deviceIP, agentConfig, httpClient)

	var snmpConfig *snmpparse.SNMPConfig
	if v := args.Get(0); v != nil {
		snmpConfig = v.(*snmpparse.SNMPConfig)
	}

	return snmpConfig, args.String(1), args.Error(2)
}
