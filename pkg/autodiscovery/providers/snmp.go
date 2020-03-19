// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package providers

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SNMPConfigProvider implements the Config Provider interface
type SNMPConfigProvider struct {
}

// NewSNMPConfigProvider returns a new SNMPConfigProvider
func NewSNMPConfigProvider(conf config.ConfigurationProviders) (ConfigProvider, error) {
	cfp := SNMPConfigProvider{}
	return cfp, nil
}

// String returns a string representation of the SNMPConfigProvider
func (cf SNMPConfigProvider) String() string {
	return names.SNMP
}

// IsUpToDate returns true
func (cf SNMPConfigProvider) IsUpToDate() (bool, error) {
	return true, nil
}

// Collect collects AD config templates from the agent configuration
func (cf SNMPConfigProvider) Collect() ([]integration.Config, error) {
	allConfigs := []integration.Config{}
	var snmpConfig util.SNMPListenerConfig
	err := config.Datadog.UnmarshalKey("snmp_listener", &snmpConfig)
	if err != nil {
		return nil, err
	}
	for i, config := range snmpConfig.Configs {
		adIdentifier := fmt.Sprintf("snmp_%d", i)
		log.Debugf("Building SNMP config for %s", adIdentifier)
		instance := "ip_address: %%host%%"
		if config.Port != 0 {
			instance = fmt.Sprintf("%s\nport: %d", instance, config.Port)
		}
		if config.Version != "" {
			instance = fmt.Sprintf("%s\nsnmp_version: %s", instance, config.Version)
		}
		if config.Timeout != 0 {
			instance = fmt.Sprintf("%s\ntimeout: %d", instance, config.Timeout)
		}
		if config.Retries != 0 {
			instance = fmt.Sprintf("%s\nretries: %d", instance, config.Retries)
		}
		if config.Community != "" {
			instance = fmt.Sprintf("%s\ncommunity_string: %s", instance, config.Community)
		}
		if config.User != "" {
			instance = fmt.Sprintf("%s\nuser: %s", instance, config.User)
		}
		if config.AuthKey != "" {
			instance = fmt.Sprintf("%s\nauthKey: %s", instance, config.AuthKey)
		}
		if config.AuthProtocol != "" {
			var authProtocol string
			if config.AuthProtocol == "MD5" {
				authProtocol = "usmHMACMD5AuthProtocol"
			} else if config.AuthProtocol == "SHA" {
				authProtocol = "usmHMACSHAAuthProtocol"
			}
			instance = fmt.Sprintf("%s\nauthProtocol: %s", instance, authProtocol)
		}
		if config.PrivKey != "" {
			instance = fmt.Sprintf("%s\nprivKey: %s", instance, config.PrivKey)
		}
		if config.PrivProtocol != "" {
			var privProtocol string
			if config.PrivProtocol == "DES" {
				privProtocol = "usmDESPrivProtocol"
			} else if config.PrivProtocol == "AES" {
				privProtocol = "usmAesCfb128Protocol"
			}
			instance = fmt.Sprintf("%s\nprivProtocol: %s", instance, privProtocol)
		}
		if config.ContextEngineID != "" {
			instance = fmt.Sprintf("%s\ncontext_engine_id: %s", instance, config.ContextEngineID)
		}
		if config.ContextName != "" {
			instance = fmt.Sprintf("%s\ncontext_name: %s", instance, config.ContextName)
		}
		instances := [][]integration.Data{{integration.Data(instance)}}
		initConfigs := [][]integration.Data{{integration.Data("")}}
		newConfigs := buildTemplates(adIdentifier, []string{"snmp"}, initConfigs, instances)
		allConfigs = append(allConfigs, newConfigs...)
	}
	return allConfigs, nil
}

func init() {
	RegisterProvider(names.SNMP, NewSNMPConfigProvider)
}
