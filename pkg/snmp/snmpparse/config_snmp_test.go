// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmpparse

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
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

func TestParseConfigSnmpMainNoConfig(t *testing.T) {
	conf := configmock.NewFromYAML(t, ``)
	_, err := parseConfigSnmpMain(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no config given for snmp_listener")
}

func TestParseConfigSnmpMainSnmpListenerPath(t *testing.T) {
	conf := configmock.NewFromYAML(t, `
snmp_listener:
  configs:
    - network_address: 10.0.0.0/24
      snmp_version: 2
      community_string: secret
`)
	output, err := parseConfigSnmpMain(conf)
	require.NoError(t, err)
	require.Len(t, output, 1)
	assert.Equal(t, "10.0.0.0/24", output[0].NetAddress)
	assert.Equal(t, "2", output[0].Version)
	assert.Equal(t, "secret", output[0].CommunityString)
}

func TestParseConfigSnmpInvalidYAML(t *testing.T) {
	type Data = integration.Data
	input := integration.Config{
		Name:      "snmp",
		Instances: []Data{Data("{{invalid yaml")},
	}
	// Should not panic, just return configs with defaults
	configs := ParseConfigSnmp(input)
	assert.Len(t, configs, 1)
}

func TestGetIPConfigMalformedCIDR(t *testing.T) {
	// Malformed CIDR should be skipped gracefully
	configs := []SNMPConfig{
		{
			NetAddress:      "not-a-cidr",
			CommunityString: "public",
		},
	}
	result := GetIPConfig("10.0.0.1", configs)
	assert.Equal(t, SNMPConfig{}, result)
}

func TestNewSNMP_V2cDefault(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress:       "192.168.1.1",
		Port:            161,
		CommunityString: "public",
		Timeout:         2,
		Retries:         3,
	}
	snmp, err := NewSNMP(conf, logger)
	require.NoError(t, err)
	assert.Equal(t, gosnmp.Version2c, snmp.Version)
	assert.Equal(t, "public", snmp.Community)
	assert.Equal(t, uint16(161), snmp.Port)
}

func TestNewSNMP_V3WithUsername(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress: "192.168.1.1",
		Port:      161,
		Timeout:   2,
		Retries:   3,
		Username:  "admin",
		AuthKey:   "authpass1234",
		PrivKey:   "privpass1234",
	}
	snmp, err := NewSNMP(conf, logger)
	require.NoError(t, err)
	assert.Equal(t, gosnmp.Version3, snmp.Version)
	assert.Equal(t, gosnmp.AuthPriv, snmp.MsgFlags)
}

func TestNewSNMP_V3ExplicitVersion(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress:    "192.168.1.1",
		Port:         161,
		Version:      "3",
		Timeout:      2,
		Retries:      3,
		Username:     "admin",
		AuthProtocol: "SHA256",
		AuthKey:      "authpass1234",
		PrivProtocol: "AES",
		PrivKey:      "privpass1234",
	}
	snmp, err := NewSNMP(conf, logger)
	require.NoError(t, err)
	assert.Equal(t, gosnmp.Version3, snmp.Version)
	params := snmp.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	assert.Equal(t, "admin", params.UserName)
	assert.Equal(t, gosnmp.SHA256, params.AuthenticationProtocol)
	assert.Equal(t, gosnmp.AES, params.PrivacyProtocol)
}

func TestNewSNMP_V3AuthNoPriv(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress: "192.168.1.1",
		Port:      161,
		Timeout:   2,
		Username:  "admin",
		AuthKey:   "authpass1234",
		// No PrivKey -> AuthNoPriv
	}
	snmp, err := NewSNMP(conf, logger)
	require.NoError(t, err)
	assert.Equal(t, gosnmp.AuthNoPriv, snmp.MsgFlags)
}

func TestNewSNMP_V3NoAuthNoPriv(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress: "192.168.1.1",
		Port:      161,
		Timeout:   2,
		Username:  "admin",
		// No AuthKey, no PrivKey -> NoAuthNoPriv
	}
	snmp, err := NewSNMP(conf, logger)
	require.NoError(t, err)
	assert.Equal(t, gosnmp.NoAuthNoPriv, snmp.MsgFlags)
}

func TestNewSNMP_V3ExplicitSecurityLevel(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress:     "192.168.1.1",
		Port:          161,
		Timeout:       2,
		Username:      "admin",
		AuthKey:       "authpass1234",
		SecurityLevel: "authNoPriv",
	}
	snmp, err := NewSNMP(conf, logger)
	require.NoError(t, err)
	assert.Equal(t, gosnmp.AuthNoPriv, snmp.MsgFlags)
}

func TestNewSNMP_ZeroTimeout(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress: "192.168.1.1",
		Timeout:   0,
	}
	_, err := NewSNMP(conf, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout cannot be 0")
}

func TestNewSNMP_InvalidVersion(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress: "192.168.1.1",
		Version:   "4",
		Timeout:   2,
	}
	_, err := NewSNMP(conf, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestNewSNMP_V3NoUsername(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress: "192.168.1.1",
		Version:   "3",
		Timeout:   2,
	}
	_, err := NewSNMP(conf, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "username is required")
}

func TestNewSNMP_InvalidAuthProtocol(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress:    "192.168.1.1",
		Version:      "3",
		Timeout:      2,
		Username:     "admin",
		AuthProtocol: "INVALID",
	}
	_, err := NewSNMP(conf, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication protocol")
}

func TestNewSNMP_InvalidPrivProtocol(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress:    "192.168.1.1",
		Version:      "3",
		Timeout:      2,
		Username:     "admin",
		PrivProtocol: "INVALID",
	}
	_, err := NewSNMP(conf, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "privacy protocol")
}

func TestNewSNMP_InvalidSecurityLevel(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress:     "192.168.1.1",
		Version:       "3",
		Timeout:       2,
		Username:      "admin",
		SecurityLevel: "invalidLevel",
	}
	_, err := NewSNMP(conf, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "security level")
}

func TestNewSNMP_DefaultPort(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress:       "192.168.1.1",
		Port:            0, // should default to 161
		CommunityString: "public",
		Timeout:         2,
	}
	snmp, err := NewSNMP(conf, logger)
	require.NoError(t, err)
	assert.Equal(t, uint16(DefaultPort), snmp.Port)
}

func TestNewSNMP_DefaultCommunityString(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress: "192.168.1.1",
		Version:   "2",
		Timeout:   2,
		// No CommunityString set - should default
	}
	_, err := NewSNMP(conf, logger)
	require.NoError(t, err)
	assert.Equal(t, DefaultCommunityString, conf.CommunityString)
}

func TestNewSNMP_V1(t *testing.T) {
	logger := logmock.New(t)
	conf := &SNMPConfig{
		IPAddress:       "192.168.1.1",
		Version:         "1",
		Timeout:         2,
		CommunityString: "public",
	}
	snmp, err := NewSNMP(conf, logger)
	require.NoError(t, err)
	assert.Equal(t, gosnmp.Version1, snmp.Version)
}
