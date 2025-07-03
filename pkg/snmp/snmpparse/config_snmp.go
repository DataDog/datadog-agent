// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package snmpparse extracts SNMP configurations from agent config data.
package snmpparse

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	snmplistener "github.com/DataDog/datadog-agent/pkg/snmp"
)

// SNMPConfig is a generic container for configuration data specific to the SNMP
// integration.
type SNMPConfig struct {

	// General
	IPAddress string `yaml:"ip_address"`
	Port      uint16 `yaml:"port"`
	Version   string `yaml:"snmp_version"`
	Timeout   int    `yaml:"timeout"`
	Retries   int    `yaml:"retries"`
	// v1 &2
	CommunityString string `yaml:"community_string"`
	// v3
	Username     string `yaml:"user"`
	AuthProtocol string `yaml:"authProtocol"`
	AuthKey      string `yaml:"authKey"`
	PrivProtocol string `yaml:"privProtocol"`
	PrivKey      string `yaml:"privKey"`
	Context      string `yaml:"context_name"`
	// network
	NetAddress string `yaml:"network_address"`
	// These are omitted from the yaml because we don't let users configure
	// them, but there are cases where we use them (e.g. the snmpwalk command)
	SecurityLevel           string `yaml:"-"`
	UseUnconnectedUDPSocket bool   `yaml:"-"`
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
	// an array containing all the snmp instances
	var snmpconfigs []SNMPConfig

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
	//the UnmarshalKey stores the result in mapstructures while the snmpconfig is in yaml
	//so for each result of the Unmarshal key we store the result in a tmp SNMPConfig{} object
	if conf.IsSet("network_devices.autodiscovery.configs") {
		err := structure.UnmarshalKey(conf, "network_devices.autodiscovery.configs", &configs, structure.ImplicitlyConvertArrayToMapSet)
		if err != nil {
			fmt.Printf("unable to get snmp config from network_devices.autodiscovery: %v", err)
			return nil, err
		}
	} else if conf.IsSet("snmp_listener.configs") {
		err := structure.UnmarshalKey(conf, "snmp_listener.configs", &configs, structure.ImplicitlyConvertArrayToMapSet)
		if err != nil {
			fmt.Printf("unable to get snmp config from snmp_listener: %v", err)
			return nil, err
		}
	} else {
		return nil, errors.New("no config given for snmp_listener")
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
func GetConfigCheckSnmp(conf config.Component, client ipc.HTTPClient) ([]SNMPConfig, error) {
	// TODO: change the URL if the snmp check is a cluster check
	// add /agent/config-check to cluster agent API
	// Copy the code from comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go#writeConfigCheck
	endpoint, err := client.NewIPCEndpoint("/agent/config-check")
	if err != nil {
		return nil, err
	}
	urlValues := url.Values{}
	urlValues.Set("raw", "true")

	res, err := endpoint.DoGet(ipchttp.WithValues(urlValues))
	if err != nil {
		return nil, err
	}

	cr := integration.ConfigCheckResponse{}
	err = json.Unmarshal(res, &cr)
	if err != nil {
		return nil, err
	}
	// Store the SNMP config in an array (snmpConfigs)
	// c is of type config while the cr is the config check response including the instances
	var snmpConfigs []SNMPConfig
	for _, c := range cr.Configs {
		if c.Config.Name == "snmp" {
			snmpConfigs = append(snmpConfigs, ParseConfigSnmp(c.Config)...)
		}
	}
	snmpconfigMain, _ := parseConfigSnmpMain(conf)
	snmpConfigs = append(snmpConfigs, snmpconfigMain...)

	return snmpConfigs, nil

}

// GetIPConfig finds the SNMPConfig for a specific IP address.
// If the IP is explicitly configured, that will be returned;
// if it isn't, but it is part of a configured subnet, then the
// subnet config will be returned. If there are no matches, this
// will return an empty SNMPConfig.
func GetIPConfig(ipAddress string, SnmpConfigList []SNMPConfig) SNMPConfig {
	var ipAddressConfigs []SNMPConfig
	var netAddressConfigs []SNMPConfig

	// split the SnmpConfigList to get the IP addresses separated from
	// the network addresses
	for _, snmpConfig := range SnmpConfigList {
		if snmpConfig.IPAddress != "" {
			ipAddressConfigs = append(ipAddressConfigs, snmpConfig)
		}
		if snmpConfig.NetAddress != "" {
			netAddressConfigs = append(netAddressConfigs, snmpConfig)
		}
	}

	// check if the ip address is explicitly mentioned
	for _, snmpIPconfig := range ipAddressConfigs {
		if snmpIPconfig.IPAddress == ipAddress {
			return snmpIPconfig
		}
	}
	// check if the ip address is a part of a network/subnet
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

// GetParamsFromAgent returns the SNMPConfig for a specific IP address, by querying the local agent.
func GetParamsFromAgent(deviceIP string, conf config.Component, client ipc.HTTPClient) (*SNMPConfig, error) {
	snmpConfigList, err := GetConfigCheckSnmp(conf, client)
	if err != nil {
		return nil, fmt.Errorf("unable to load SNMP config from agent: %w", err)
	}
	instance := GetIPConfig(deviceIP, snmpConfigList)
	if instance.IPAddress != "" {
		instance.IPAddress = deviceIP
		return &instance, nil
	}
	return nil, fmt.Errorf("agent has no SNMP config for IP %s", deviceIP)
}
