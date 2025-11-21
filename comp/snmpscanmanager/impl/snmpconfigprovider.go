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
	GetDeviceConfig(deviceIP string, agentConfig config.Component, httpClient ipc.HTTPClient) (*snmpparse.SNMPConfig, string, error)
}

type snmpConfigProviderImpl struct {
}

func newSnmpConfigProvider() snmpConfigProvider {
	return &snmpConfigProviderImpl{}
}

func (p *snmpConfigProviderImpl) GetDeviceConfig(deviceIP string, agentConfig config.Component, httpClient ipc.HTTPClient) (*snmpparse.SNMPConfig, string, error) {
	return snmpparse.GetParamsFromAgent(deviceIP, agentConfig, httpClient)
}
