// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package providers

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestSNMPConfigProvider(t *testing.T) {
	snmpConfig := util.SNMPConfig{
		Network:   "192.168.0.0/24",
		Community: "public",
		Port:      1234,
		Version:   "2",
		Timeout:   5,
		Retries:   3,
	}
	listenerConfig := util.SNMPListenerConfig{
		Configs: []util.SNMPConfig{snmpConfig},
	}

	mockConfig := config.Mock()
	mockConfig.Set("snmp_listener", listenerConfig)

	p := SNMPConfigProvider{}

	configs, err := p.Collect()
	assert.Equal(t, nil, err)

	assert.Equal(t, 1, len(configs))
	assert.Equal(t, 1, len(configs[0].Instances))
	assert.Equal(t, "ip_address: %%host%%\nport: 1234\nsnmp_version: 2\ntimeout: 5\nretries: 3\ncommunity_string: public", string(configs[0].Instances[0]))
}

func TestSNMPConfigProviderV3(t *testing.T) {
	snmpConfig := util.SNMPConfig{
		Network:      "192.168.0.0/24",
		Version:      "3",
		User:         "admin",
		AuthKey:      "secret",
		AuthProtocol: "SHA",
		PrivKey:      "privSecret",
		PrivProtocol: "AES",
	}
	listenerConfig := util.SNMPListenerConfig{
		Configs: []util.SNMPConfig{snmpConfig},
	}

	mockConfig := config.Mock()
	mockConfig.Set("snmp_listener", listenerConfig)

	p := SNMPConfigProvider{}

	configs, err := p.Collect()
	assert.Equal(t, nil, err)

	assert.Equal(t, 1, len(configs))
	assert.Equal(t, 1, len(configs[0].Instances))
	assert.Equal(t, "ip_address: %%host%%\nsnmp_version: 3\nuser: admin\nauthKey: secret\nauthProtocol: usmHMACSHAAuthProtocol\nprivKey: privSecret\nprivProtocol: usmAesCfb128Protocol", string(configs[0].Instances[0]))
}
