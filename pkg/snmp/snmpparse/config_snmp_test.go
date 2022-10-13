// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmpparse

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

func TestOneInstance(t *testing.T) {
	//define the input
	type Data = integration.Data
	input := integration.Config{
		Name:      "snmp",
		Instances: []Data{Data("{\"ip_address\":\"98.6.18.158\",\"port\":161,\"community_string\":\"password\",\"snmp_version\":\"2\",\"timeout\":60,\"retries\":3}")},
	}
	//define the output
	Exoutput := []SNMPConfig{
		{
			Version:         "2",
			CommunityString: "password",
			IPAddress:       "98.6.18.158",
			Port:            161,
			Timeout:         60,
			Retries:         3,
		},
	}
	assertSNMP(t, input, Exoutput)
}
func TestSeveralInstances(t *testing.T) {
	//define the input
	type Data = integration.Data
	input := integration.Config{
		Name: "snmp",
		Instances: []Data{Data("{\"ip_address\":\"98.6.18.158\",\"port\":161,\"community_string\":\"password\",\"snmp_version\":\"2\",\"timeout\":60,\"retries\":3}"),
			Data("{\"ip_address\":\"98.6.18.159\",\"port\":162,\"community_string\":\"drowssap\",\"snmp_version\":\"2\",\"timeout\":30,\"retries\":5}")},
	}
	//define the output
	Exoutput := []SNMPConfig{
		{
			Version:         "2",
			CommunityString: "password",
			IPAddress:       "98.6.18.158",
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
			Version:         "2",
			CommunityString: "password",
			IPAddress:       "98.6.18.158",
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
	input := "98.6.18.160"
	Exoutput := SNMPConfig{
		Version:         "3",
		CommunityString: "drowssap",
		IPAddress:       "98.6.18.160",
		Port:            172,
		Timeout:         30,
		Retries:         5,
	}
	assertIP(t, input, IPList, Exoutput)

}

func assertIP(t *testing.T, input string, snmpConfigList []SNMPConfig, expectedOutput SNMPConfig) {
	output := GetIPConfig(input, snmpConfigList)
	assert.Equal(t, expectedOutput, output)
}
