// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"net/url"
	"testing"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/stretchr/testify/suite"
)

type YamlConfigTestSuite struct {
	suite.Suite
	config *coreConfig.MockConfig
}

func (suite *YamlConfigTestSuite) SetupTest() {
	suite.config = coreConfig.Mock()
}

func (suite *YamlConfigTestSuite) TestExtractNetworkDevicesDDNetworkDevicesUrl() {
	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("network_devices.network_devices_dd_url", "https://orchestrator-link.com")
	actual, err := extractNetworkDevicesDDUrl()
	suite.NoError(err)
	expected, err := url.Parse("https://orchestrator-link.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractNetworkDevicesDDNonSet() {
	actual, err := extractNetworkDevicesDDUrl()
	suite.NoError(err)
	expected, err := url.Parse("https://network-devices.datadoghq.com")
	suite.NoError(err)
	suite.Equal(expected, actual)
}

func (suite *YamlConfigTestSuite) TestExtractNetworkDevicesEndpoints() {
	expected := make(map[string]string)
	expected["key1"] = "endpoint1.com"
	expected["key2"] = "endpoint2.com"
	expected["key3"] = "endpoint2.com"
	expected["apikey_20"] = "network-devices.datadoghq.com"
	var actualEndpoints []apicfg.Endpoint

	suite.config.Set("api_key", "wassupkey")
	suite.config.Set("network_devices.network_devices_additional_endpoints", `{"https://endpoint1.com": ["key1"], "https://endpoint2.com": ["key2", "key3"]}`)
	err := extractNetworkDevicesAdditionalEndpoints(&url.URL{}, &actualEndpoints)
	suite.NoError(err)
	suite.Equal(3, len(actualEndpoints))
	for _, actual := range actualEndpoints {
		suite.Equal(expected[actual.APIKey], actual.Endpoint.Hostname())
	}
}

func TestYamlConfigTestSuite(t *testing.T) {
	suite.Run(t, new(YamlConfigTestSuite))
}
