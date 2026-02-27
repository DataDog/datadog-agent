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

	yaml "go.yaml.in/yaml/v2"

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
	// Network
	NetAddress string `yaml:"network_address"`
	// These are omitted from the yaml because we don't let users configure
	// them, but there are cases where we use them (e.g. the snmpwalk command)
	SecurityLevel           string `yaml:"-"`
	UseUnconnectedUDPSocket bool   `yaml:"-"`

	// NamespaceInternal is for internal use only and should not be used outside of this package.
	NamespaceInternal string `yaml:"namespace"`
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
	var snmpConfigs []SNMPConfig

	for _, inst := range c.Instances {
		instance := SNMPConfig{}
		SetDefault(&instance)
		err := yaml.Unmarshal(inst, &instance)
		if err != nil {
			fmt.Printf("unable to get snmp config: %v", err)
		}
		// add the instance(type SNMPConfig) to the array snmpConfigs
		snmpConfigs = append(snmpConfigs, instance)
	}

	return snmpConfigs
}

func parseConfigSnmpMain(conf config.Component) ([]SNMPConfig, error) {
	snmpConfigs := []SNMPConfig{}
	configs := []snmplistener.UnmarshalledConfig{}

	// the UnmarshalKey stores the result in mapstructures while the snmpConfig is in yaml
	// so for each result of the Unmarshal key we store the result in a tmp SNMPConfig{} object
	if conf.IsConfigured("network_devices.autodiscovery.configs") {
		err := structure.UnmarshalKey(conf, "network_devices.autodiscovery.configs", &configs, structure.ImplicitlyConvertArrayToMapSet)
		if err != nil {
			fmt.Printf("unable to get snmp config from network_devices.autodiscovery: %v", err)
			return nil, err
		}
	} else if conf.IsConfigured("snmp_listener.configs") {
		err := structure.UnmarshalKey(conf, "snmp_listener.configs", &configs, structure.ImplicitlyConvertArrayToMapSet)
		if err != nil {
			fmt.Printf("unable to get snmp config from snmp_listener: %v", err)
			return nil, err
		}
	} else {
		return nil, errors.New("no config given for snmp_listener")
	}

	for c := range configs {
		snmpConfig := SNMPConfig{}
		SetDefault(&snmpConfig)

		snmpConfig.NetAddress = configs[c].Network
		snmpConfig.Port = configs[c].Port
		snmpConfig.Version = configs[c].Version
		snmpConfig.Timeout = configs[c].Timeout
		snmpConfig.Retries = configs[c].Retries
		snmpConfig.CommunityString = configs[c].Community
		snmpConfig.Username = configs[c].User
		snmpConfig.AuthProtocol = configs[c].AuthProtocol
		snmpConfig.AuthKey = configs[c].AuthKey
		snmpConfig.PrivProtocol = configs[c].PrivProtocol
		snmpConfig.PrivKey = configs[c].PrivKey
		snmpConfig.Context = configs[c].ContextName
		snmpConfig.NamespaceInternal = configs[c].Namespace

		snmpConfigs = append(snmpConfigs, snmpConfig)
	}

	return snmpConfigs, nil
}

// GetConfigCheckSnmp returns each SNMPConfig for all running config checks, by querying the local agent.
// If the agent isn't running or is unreachable, this will fail.
func GetConfigCheckSnmp(conf config.Component, client ipc.HTTPClient) ([]SNMPConfig, error) {
	// TODO: change the URL if the SNMP check is a cluster check
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
	snmpConfigMain, _ := parseConfigSnmpMain(conf)
	snmpConfigs = append(snmpConfigs, snmpConfigMain...)

	return snmpConfigs, nil
}

// GetIPConfig finds the SNMPConfig for a specific IP address.
// If the IP is explicitly configured, that will be returned;
// if it isn't, but it is part of a configured subnet, then the
// subnet config will be returned. If there are no matches, this
// will return an empty SNMPConfig.
func GetIPConfig(ipAddress string, snmpConfigList []SNMPConfig) SNMPConfig {
	var ipAddressConfigs []SNMPConfig
	var netAddressConfigs []SNMPConfig

	// split the snmpConfigList to get the IP addresses separated from
	// the network addresses
	for _, snmpConfig := range snmpConfigList {
		if snmpConfig.IPAddress != "" {
			ipAddressConfigs = append(ipAddressConfigs, snmpConfig)
		}
		if snmpConfig.NetAddress != "" {
			netAddressConfigs = append(netAddressConfigs, snmpConfig)
		}
	}

	// check if the ip address is explicitly mentioned
	for _, snmpIPConfig := range ipAddressConfigs {
		if snmpIPConfig.IPAddress == ipAddress {
			return snmpIPConfig
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
func GetParamsFromAgent(deviceIP string, conf config.Component, client ipc.HTTPClient) (*SNMPConfig, string, error) {
	snmpConfigList, err := GetConfigCheckSnmp(conf, client)
	if err != nil {
		return nil, "", fmt.Errorf("unable to load SNMP config from agent: %w", err)
	}
	instance := GetIPConfig(deviceIP, snmpConfigList)
	namespace := instance.NamespaceInternal
	if namespace == "" {
		namespace = conf.GetString("network_devices.namespace")
	}
	if instance.IPAddress != "" {
		instance.IPAddress = deviceIP
		return &instance, namespace, nil
	}
	return nil, "", fmt.Errorf("agent has no SNMP config for IP %s", deviceIP)
}
