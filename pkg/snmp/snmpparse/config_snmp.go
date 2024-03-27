// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package snmpparse extracts SNMP configurations from agent config data.
package snmpparse

import (
	"encoding/json"
	"fmt"
	"net"
	"reflect"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/response"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	snmplistener "github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/viper"
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

// SetDefault sets the standard default config values
func SetDefault(sc *SNMPConfig) {
	sc.Port = 161
	sc.Version = ""
	sc.Timeout = 2
	sc.Retries = 3

}

// ParseConfigSnmp extracts all SNMPConfigs from an autodiscovery config.
// Any loading errors are logged but not returned.
func ParseConfigSnmp(c integration.Config) []SNMPConfig {
	//an array containing all the snmp instances
	snmpconfigs := []SNMPConfig{}

	for _, inst := range c.Instances {
		instance := SNMPConfig{}
		SetDefault(&instance)
		err := yaml.Unmarshal(inst, &instance)
		if err != nil {
			fmt.Printf("unable to get snmp config: %v", err)
		}
		// add the instance(type SNMPConfig) to the array snmpconfigs
		snmpconfigs = append(snmpconfigs, instance)
	}

	return snmpconfigs
}

func parseConfigSnmpMain(conf config.Component) ([]SNMPConfig, error) {
	snmpconfigs := []SNMPConfig{}
	configs := []snmplistener.Config{}
	opt := viper.DecodeHook(
		func(rf reflect.Kind, rt reflect.Kind, data interface{}) (interface{}, error) {
			// Turn an array into a map for ignored addresses
			if rf != reflect.Slice {
				return data, nil
			}
			if rt != reflect.Map {
				return data, nil
			}
			newData := map[interface{}]bool{}
			for _, i := range data.([]interface{}) {
				newData[i] = true
			}
			return newData, nil
		},
	)
	//the UnmarshalKey stores the result in mapstructures while the snmpconfig is in yaml
	//so for each result of the Unmarshal key we storre the result in a tmp SNMPConfig{} object
	err := conf.UnmarshalKey("snmp_listener.configs", &configs, opt)
	if err != nil {
		fmt.Printf("unable to get snmp config from snmp_listener: %v", err)
		return nil, err
	}
	for c := range configs {
		snmpconfig := SNMPConfig{}
		SetDefault(&snmpconfig)

		snmpconfig.NetAddress = configs[c].Network
		snmpconfig.Port = configs[c].Port
		snmpconfig.Version = configs[c].Version
		snmpconfig.Timeout = configs[c].Timeout
		snmpconfig.Retries = configs[c].Retries
		snmpconfig.CommunityString = configs[c].Community
		snmpconfig.Username = configs[c].User
		snmpconfig.AuthProtocol = configs[c].AuthProtocol
		snmpconfig.AuthKey = configs[c].AuthKey
		snmpconfig.PrivProtocol = configs[c].PrivProtocol
		snmpconfig.PrivKey = configs[c].PrivKey
		snmpconfig.Context = configs[c].ContextName

		snmpconfigs = append(snmpconfigs, snmpconfig)

	}

	return snmpconfigs, nil

}

// GetConfigCheckSnmp returns each SNMPConfig for all running config checks, by querying the local agent.
// If the agent isn't running or is unreachable, this will fail.
func GetConfigCheckSnmp(conf config.Component) ([]SNMPConfig, error) {

	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	err := util.SetAuthToken(conf)
	if err != nil {
		return nil, err
	}
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(conf)
	if err != nil {
		return nil, err
	}
	//TODO: change the configCheckURLSnmp if the snmp check is a cluster check
	if configCheckURLSnmp == "" {
		configCheckURLSnmp = fmt.Sprintf("https://%v:%v/agent/config-check", ipcAddress, conf.GetInt("cmd_port"))
	}
	r, err := util.DoGet(c, configCheckURLSnmp, util.LeaveConnectionOpen)
	if err != nil {
		return nil, err
	}
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
	snmpconfigMain, _ := parseConfigSnmpMain(conf)
	snmpconfigs = append(snmpconfigs, snmpconfigMain...)

	return snmpconfigs, nil

}

// GetIPConfig finds the SNMPConfig for a specific IP address.
// If the IP is explicitly configured, that will be returned;
// if it isn't, but it is part of a configured subnet, then the
// subnet config will be returned. If there are no matches, this
// will return an empty SNMPConfig.
func GetIPConfig(ipAddress string, SnmpConfigList []SNMPConfig) SNMPConfig {
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
		if snmpIPconfig.IPAddress == ipAddress {
			return snmpIPconfig
		}
	}
	//check if the ip address is a part of a network/subnet
	for _, snmpNetConfig := range netAddressConfigs {
		_, subnet, err := net.ParseCIDR(snmpNetConfig.NetAddress)
		if err != nil {
			// ignore any malformed configs
			continue
		}
		ip := net.ParseIP(ipAddress)
		if subnet.Contains(ip) {
			snmpNetConfig.IPAddress = ipAddress
			return snmpNetConfig
		}
	}
	return SNMPConfig{}
}
