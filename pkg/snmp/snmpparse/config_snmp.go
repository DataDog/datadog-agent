// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package snmpparse

import (
	"encoding/json"
	"fmt"
	"net"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var configCheckURLSnmp string

// SNMPConfig is a generic container for configuration data specific to the SNMP
// integration.

type SNMPConfig struct {

	//General
	IPAddress string `yaml:"ip_address"`
	Port      uint16 `yaml:"port"`
	Version   string `yaml:"snmp_version"`
	Timeout   int    `yaml:"timeout"`
	Retries   int    `yaml:"retries"`
	//v1 &2
	CommunityString string `yaml:"community_string"`
	//v3
	Username     string `yaml:"user"`
	AuthProtocol string `yaml:"authProtocol"`
	AuthKey      string `yaml:"authKey"`
	PrivProtocol string `yaml:"privProtocol"`
	PrivKey      string `yaml:"privKey"`
	Context      string `yaml:"context_name"`
	//network
	NetAddress string `yaml:"network_address"`
}

func ParseConfigSnmp(c integration.Config) []SNMPConfig {
	//an array containing all the snmp instances
	snmpconfigs := []SNMPConfig{}

	for _, inst := range c.Instances {
		instance := SNMPConfig{}
		err := yaml.Unmarshal(inst, &instance)
		if err != nil {
			fmt.Printf("unable to get snmp config: %v", err)
		}
		// add the instance(type SNMPConfig) to the array snmpconfigs
		snmpconfigs = append(snmpconfigs, instance)
	}

	return snmpconfigs
}

func GetConfigCheckSnmp() ([]SNMPConfig, error) {

	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	err := util.SetAuthToken()
	if err != nil {
		return nil, err
	}
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}
	//TODO: change the configCheckURLSnmp if the snmp check is a cluster check
	if configCheckURLSnmp == "" {
		configCheckURLSnmp = fmt.Sprintf("https://%v:%v/agent/config-check", ipcAddress, config.Datadog.GetInt("cmd_port"))
	}
	r, _ := util.DoGet(c, configCheckURLSnmp, util.LeaveConnectionOpen)
	cr := response.ConfigCheckResponse{}
	err = json.Unmarshal(r, &cr)
	if err != nil {
		return nil, err
	}
	//Store the SNMP config in an array (snmpconfigs)
	//c is of type config while the cr is the config check response including the instances
	snmpconfigs := []SNMPConfig{}
	for _, c := range cr.Configs {
		if c.Name == "snmp" {
			snmpconfigs = append(snmpconfigs, ParseConfigSnmp(c)...)
		}
	}

	return snmpconfigs, nil

}

func GetIPConfig(ip_address string, SnmpConfigList []SNMPConfig) SNMPConfig {
	ipAddressConfigs := []SNMPConfig{}
	netAddressConfigs := []SNMPConfig{}

	//split the SnmpConfigList to get the IP addresses separated from
	//the network addresses
	for _, snmpconfig := range SnmpConfigList {
		if snmpconfig.IPAddress != "" {
			ipAddressConfigs = append(ipAddressConfigs, snmpconfig)
		}
		if snmpconfig.NetAddress != "" {
			netAddressConfigs = append(netAddressConfigs, snmpconfig)
		}
	}

	//check if the ip address is explicitly mentioned
	for _, snmpIPconfig := range ipAddressConfigs {
		if snmpIPconfig.IPAddress == ip_address {
			return snmpIPconfig
		}
	}
	//check if the ip address is a part of a network/subnet
	for _, snmpNetConfig := range netAddressConfigs {
		_, subnet, _ := net.ParseCIDR(snmpNetConfig.NetAddress)
		ip := net.ParseIP(ip_address)
		if subnet.Contains(ip) {
			snmpNetConfig.IPAddress = ip_address
			return snmpNetConfig
		}

	}

	return SNMPConfig{}
}
