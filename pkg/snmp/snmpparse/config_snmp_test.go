// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmpparse

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestOneInstance(t *testing.T) {
	// define the input
	type Data = integration.Data
	input := integration.Config{
		Name:      "snmp",
		Instances: []Data{Data("{\"ip_address\":\"98.6.18.158\",\"namespace\":\"my_namespace\",\"port\":161,\"community_string\":\"password\",\"snmp_version\":\"2\",\"timeout\":60,\"retries\":3}")},
	}
	// define the output
	Exoutput := []SNMPConfig{
		{
			Version:           "2",
			CommunityString:   "password",
			IPAddress:         "98.6.18.158",
			Port:              161,
			Timeout:           60,
			Retries:           3,
			NamespaceInternal: "my_namespace",
		},
	}
	assertSNMP(t, input, Exoutput)
}

func TestDefaultSet(t *testing.T) {
	// define the input
	type Data = integration.Data
	input := integration.Config{
		Name:      "snmp",
		Instances: []Data{Data("{\"ip_address\":\"98.6.18.158\"}")},
	}
	// define the output
	Exoutput := []SNMPConfig{
		{
			Version:   "",
			IPAddress: "98.6.18.158",
			Port:      161,
			Timeout:   2,
			Retries:   3,
		},
	}
	assertSNMP(t, input, Exoutput)
}

func TestSeveralInstances(t *testing.T) {
	// define the input
	type Data = integration.Data
	input := integration.Config{
		Name: "snmp",
		Instances: []Data{Data("{\"ip_address\":\"98.6.18.158\",\"namespace\":\"my_namespace1\",\"port\":161,\"community_string\":\"password\",\"snmp_version\":\"2\",\"timeout\":60,\"retries\":3}"),
			Data("{\"ip_address\":\"98.6.18.159\",\"namespace\":\"my_namespace2\",\"port\":162,\"community_string\":\"drowssap\",\"snmp_version\":\"2\",\"timeout\":30,\"retries\":5}")},
	}
	// define the output
	Exoutput := []SNMPConfig{
		{
			Version:           "2",
			CommunityString:   "password",
			IPAddress:         "98.6.18.158",
			Port:              161,
			Timeout:           60,
			Retries:           3,
			NamespaceInternal: "my_namespace1",
		},
		{
			Version:           "2",
			CommunityString:   "drowssap",
			IPAddress:         "98.6.18.159",
			Port:              162,
			Timeout:           30,
			Retries:           5,
			NamespaceInternal: "my_namespace2",
		},
	}
	assertSNMP(t, input, Exoutput)
}

func assertSNMP(t *testing.T, input integration.Config, expectedOutput []SNMPConfig) {
	output := ParseConfigSnmp(input)
	assert.Equal(t, expectedOutput, output)
}

func TestGetSNMPConfig(t *testing.T) {
	IPList := []SNMPConfig{
		{
			Version:           "2",
			CommunityString:   "password",
			IPAddress:         "98.6.18.158",
			Port:              161,
			Timeout:           60,
			Retries:           3,
			NamespaceInternal: "my_namespace1",
		},
		{
			Version:           "2",
			CommunityString:   "drowssap",
			IPAddress:         "98.6.18.159",
			Port:              162,
			Timeout:           30,
			Retries:           5,
			NamespaceInternal: "my_namespace2",
		},
		{
			Version:           "3",
			CommunityString:   "drowssap",
			IPAddress:         "98.6.18.160",
			Port:              172,
			Timeout:           30,
			Retries:           5,
			NamespaceInternal: "my_namespace3",
		},
	}
	input := "98.6.18.160"
	expectedOutput := SNMPConfig{
		Version:           "3",
		CommunityString:   "drowssap",
		IPAddress:         "98.6.18.160",
		Port:              172,
		Timeout:           30,
		Retries:           5,
		NamespaceInternal: "my_namespace3",
	}
	assertIP(t, input, IPList, expectedOutput)
}

func TestGetSNMPConfigNetwork(t *testing.T) {
	IPList := []SNMPConfig{
		{
			Version:         "2",
			CommunityString: "password",
			NetAddress:      "192.168.5.0/24",
			Port:            161,
			Timeout:         60,
			Retries:         3,
		},
		{
			Version:         "2",
			CommunityString: "drowssap",
			IPAddress:       "98.6.18.159",
			Port:            162,
			Timeout:         30,
			Retries:         5,
		},
		{
			Version:         "3",
			CommunityString: "drowssap",
			IPAddress:       "98.6.18.160",
			Port:            172,
			Timeout:         30,
			Retries:         5,
		},
	}
	input := "192.168.5.3"
	expectedOutput := SNMPConfig{
		Version:         "2",
		CommunityString: "password",
		IPAddress:       "192.168.5.3",
		NetAddress:      "192.168.5.0/24",
		Port:            161,
		Timeout:         60,
		Retries:         3,
	}
	assertIP(t, input, IPList, expectedOutput)
}

func TestGetSNMPConfigNet(t *testing.T) {
	// if the ip address is a part of network but alos is defined indivudualy
	// the ip_address field should be the one that works
	IPList := []SNMPConfig{
		{
			Version:         "2",
			CommunityString: "password",
			NetAddress:      "192.168.5.0/24",
			Port:            161,
			Timeout:         60,
			Retries:         3,
		},
		{
			Version:         "2",
			CommunityString: "drowssap",
			IPAddress:       "98.6.18.159",
			Port:            162,
			Timeout:         30,
			Retries:         5,
		},
		{
			Version:         "2",
			CommunityString: "password",
			IPAddress:       "192.168.5.1",
			Port:            161,
			Timeout:         60,
			Retries:         3,
		},
	}
	input := "192.168.5.1"
	expectedOutput := SNMPConfig{
		Version:         "2",
		CommunityString: "password",
		IPAddress:       "192.168.5.1",
		Port:            161,
		Timeout:         60,
		Retries:         3,
	}
	assertIP(t, input, IPList, expectedOutput)
}

func TestGetSNMPConfigNoAddress(t *testing.T) {
	// if the ip address doesn't match anything
	IPList := []SNMPConfig{
		{
			Version:         "2",
			CommunityString: "password",
			NetAddress:      "192.168.5.0/24",
			Port:            161,
			Timeout:         60,
			Retries:         3,
		},
		{
			Version:         "2",
			CommunityString: "drowssap",
			IPAddress:       "98.6.18.159",
			Port:            162,
			Timeout:         30,
			Retries:         5,
		},
		{
			Version:         "2",
			CommunityString: "password",
			IPAddress:       "192.168.5.1",
			Port:            161,
			Timeout:         60,
			Retries:         3,
		},
	}
	input := "192.168.6.1"
	expectedOutput := SNMPConfig{}
	assertIP(t, input, IPList, expectedOutput)
}

func TestGetSNMPConfigEmpty(t *testing.T) {
	// if the snmp configuration is empty
	var IPList []SNMPConfig
	input := "192.168.6.4"
	expectedOutput := SNMPConfig{}
	assertIP(t, input, IPList, expectedOutput)
}

func TestGetSNMPConfigDefault(t *testing.T) {
	// check if the default setter is valid
	input := SNMPConfig{}
	SetDefault(&input)
	expectedOutput := SNMPConfig{
		Version: "",
		Port:    161,
		Timeout: 2,
		Retries: 3,
	}
	assert.Equal(t, expectedOutput, input)
}

func assertIP(t *testing.T, input string, snmpConfigList []SNMPConfig, expectedOutput SNMPConfig) {
	output := GetIPConfig(input, snmpConfigList)
	assert.Equal(t, expectedOutput, output)
}

func TestParseConfigSnmpMain(t *testing.T) {
	conf := configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
      - network_address: 127.0.0.1/30
        snmp_version: 1
        community_string: public
      - network_address: 127.0.0.2/30
        snmp_version: 2
        community_string: publicX
      - network_address: 127.0.0.4/30
        snmp_version: 3`,
	)

	output, err := parseConfigSnmpMain(conf)
	require.NoError(t, err)
	expectedOutput := []SNMPConfig{
		{
			Version:         "1",
			CommunityString: "public",
			NetAddress:      "127.0.0.1/30",
		},
		{
			Version:         "2",
			CommunityString: "publicX",
			NetAddress:      "127.0.0.2/30",
		},
		{
			Version:    "3",
			NetAddress: "127.0.0.4/30",
		},
	}
	assert.Equal(t, expectedOutput, output)
}

func TestIPDecodeHook(t *testing.T) {
	conf := configmock.NewFromYAML(t, `
network_devices:
  autodiscovery:
    configs:
      - network_address: 127.0.0.1/30
        snmp_version: 1
        community_string: public
        ignored_ip_addresses:
          - 10.0.1.0
          - 10.0.1.1
`)
	Output, err := parseConfigSnmpMain(conf)
	require.NoError(t, err)
	expectedOutput := []SNMPConfig{
		{
			Version:         "1",
			CommunityString: "public",
			NetAddress:      "127.0.0.1/30",
		},
	}
	assert.Equal(t, expectedOutput, Output)
}
