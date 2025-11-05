// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
)

type snmpConfigProvider interface {
	GetConfigFromAgent(deviceIP string, agentConfig config.Component, httpClient ipc.HTTPClient) (*snmpparse.SNMPConfig, error)
}

type snmpConfigProviderImpl struct {
}

func newSnmpConfigProvider() snmpConfigProvider {
	return &snmpConfigProviderImpl{}
}

func (p *snmpConfigProviderImpl) GetConfigFromAgent(deviceIP string, agentConfig config.Component, httpClient ipc.HTTPClient) (*snmpparse.SNMPConfig, error) {
	return snmpparse.GetParamsFromAgent(deviceIP, agentConfig, httpClient)
}

type snmpConfigProviderMock struct {
	configsByIP map[string]*snmpparse.SNMPConfig
	errorsByIP  map[string]error
}

func newSnmpConfigProviderMock(configsByIP map[string]*snmpparse.SNMPConfig, errorsByIP map[string]error) snmpConfigProvider {
	return &snmpConfigProviderMock{
		configsByIP: configsByIP,
		errorsByIP:  errorsByIP,
	}
}

func (p *snmpConfigProviderMock) GetConfigFromAgent(deviceIP string, _ config.Component, _ ipc.HTTPClient) (*snmpparse.SNMPConfig, error) {
	return p.configsByIP[deviceIP], p.errorsByIP[deviceIP]
}
