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
		Instances: []Data{Data("{\"ip_address\":\"98.6.18.158\",\"port\":\"161\",\"community_string\":\"password\",\"snmp_version\":\"2\",\"timeout\":\"60\",\"retries\":\"3\"}")},
	}
	//define the output
	Exoutput := DataSNMP{
		SNMPConfig{
			SnmpVersion:         "2",
			SnmpCommunityString: "password",
			SnmpIPAddress:       "98.6.18.158",
			SnmpPort:            "161",
			SnmpTimeout:         "60",
			SnmpRetries:         "3",
		},
	}
	assertSNMP(t, input, Exoutput)
}
func TestSeveralInstances(t *testing.T) {
	//define the input
	type Data = integration.Data
	input := integration.Config{
		Name: "snmp",
		Instances: []Data{Data("{\"ip_address\":\"98.6.18.158\",\"port\":\"161\",\"community_string\":\"password\",\"snmp_version\":\"2\",\"timeout\":\"60\",\"retries\":\"3\"}"),
			Data("{\"ip_address\":\"98.6.18.159\",\"port\":\"162\",\"community_string\":\"drowssap\",\"snmp_version\":\"2\",\"timeout\":\"30\",\"retries\":\"5\"}")},
	}
	//define the output
	Exoutput := DataSNMP{
		SNMPConfig{
			SnmpVersion:         "2",
			SnmpCommunityString: "password",
			SnmpIPAddress:       "98.6.18.158",
			SnmpPort:            "161",
			SnmpTimeout:         "60",
			SnmpRetries:         "3",
		},
		{
			SnmpVersion:         "2",
			SnmpCommunityString: "drowssap",
			SnmpIPAddress:       "98.6.18.159",
			SnmpPort:            "162",
			SnmpTimeout:         "30",
			SnmpRetries:         "5",
		},
	}
	assertSNMP(t, input, Exoutput)
}

func assertSNMP(t *testing.T, input integration.Config, expectedOutput DataSNMP) {
	output := ParseConfigSnmp(input)
	assert.Equal(t, expectedOutput, output)
}
