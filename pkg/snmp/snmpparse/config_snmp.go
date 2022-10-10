// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package snmpparse

import (
	"encoding/json"
	"fmt"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var configCheckURLSnmp string

type ShouldCloseConnectionSnmp int

// DataSNMP contains all snmp instances YAML code
type DataSNMP []SNMPConfig

// SNMPConfig is a generic container for configuration data specific to the SNMP
// integration.
// The DataSNMP is an array of the SNMPConfig containers

type SNMPConfig struct {

	//General
	SnmpIPAddress string `yaml:"ip_address"`
	SnmpPort      uint16 `yaml:"port"`
	SnmpVersion   string `yaml:"snmp_version"`
	SnmpTimeout   int    `yaml:"timeout"`
	SnmpRetries   int    `yaml:"retries"`
	//v1 &2
	SnmpCommunityString string `yaml:"community_string"`
	//v3
	SnmpUsername     string `yaml:"user"`
	SnmpAuthProtocol string `yaml:"authProtocol"`
	SnmpAuthKey      string `yaml:"authKey"`
	SnmpPrivProtocol string `yaml:"privProtocol"`
	SnmpPrivKey      string `yaml:"privKey"`
	SnmpContext      string `yaml:"context_name"`
}

const (
	// LeaveConnectionOpenSnmp keeps the underlying connection open after reading the request response
	LeaveConnectionOpenSnmp ShouldCloseConnectionSnmp = iota
	// CloseConnection closes the underlying connection after reading the request response
	CloseConnectionSnmp
)

func ParseConfigSnmp(c integration.Config) DataSNMP {
	//an array containing all the snmp instances
	ws := DataSNMP{}

	for _, inst := range c.Instances {
		instance := SNMPConfig{}
		err := yaml.Unmarshal(inst, &instance)
		if err != nil {
			fmt.Printf("unable to get snmp config: %v", err)
		}
		// add the instance(type SNMPConfig) to the array ws
		ws = append(ws, instance)
	}

	return ws
}

func GetConfigCheckSnmp() (DataSNMP, error) {
	ws := DataSNMP{}

	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	err := util.SetAuthToken()
	if err != nil {
		return ws, err
	}
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return ws, err
	}
	//To Do: change the configCheckURLSnmp if the snmp check is a cluster check
	if configCheckURLSnmp == "" {
		configCheckURLSnmp = fmt.Sprintf("https://%v:%v/agent/config-check", ipcAddress, config.Datadog.GetInt("cmd_port"))
	}
	r, _ := util.DoGet(c, configCheckURLSnmp, util.LeaveConnectionOpen)
	cr := response.ConfigCheckResponse{}
	err = json.Unmarshal(r, &cr)
	if err != nil {
		return ws, err
	}
	//Store the SNMP config in the DataSNMP array (ws)
	//c is of type config while the cr is the config check response including the instances
	for _, c := range cr.Configs {
		if c.Name == "snmp" {
			ws = ParseConfigSnmp(c)
			//ParseConfigSnmp(c)

		}
	}

	return ws, nil

}
func GetIPConfig(ip_address string, ip_list DataSNMP) SNMPConfig {
	//How to unit test without providing the api key ?
	//ip_list, _ = GetConfigCheckSnmp()
	instance := SNMPConfig{}
	for _, w := range ip_list {
		if w.SnmpIPAddress == ip_address {
			instance = w
		}
	}
	return instance
}
