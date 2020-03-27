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
	for i, conf := range snmpConfig.Configs {
		adIdentifier := fmt.Sprintf("snmp_%d", i)
		log.Debugf("Building SNMP config for %s", adIdentifier)
		instance := "ip_address: %%host%%"
		if conf.Port != 0 {
			instance = fmt.Sprintf("%s\nport: %d", instance, conf.Port)
		}
		if conf.Version != "" {
			instance = fmt.Sprintf("%s\nsnmp_version: %s", instance, conf.Version)
		}
		if conf.Timeout != 0 {
			instance = fmt.Sprintf("%s\ntimeout: %d", instance, conf.Timeout)
		}
		if conf.Retries != 0 {
			instance = fmt.Sprintf("%s\nretries: %d", instance, conf.Retries)
		}
		if conf.Community != "" {
			instance = fmt.Sprintf("%s\ncommunity_string: %s", instance, conf.Community)
		}
		if conf.User != "" {
			instance = fmt.Sprintf("%s\nuser: %s", instance, conf.User)
		}
		if conf.AuthKey != "" {
			instance = fmt.Sprintf("%s\nauthKey: %s", instance, conf.AuthKey)
		}
		if conf.AuthProtocol != "" {
			var authProtocol string
			if conf.AuthProtocol == "MD5" {
				authProtocol = "usmHMACMD5AuthProtocol"
			} else if conf.AuthProtocol == "SHA" {
				authProtocol = "usmHMACSHAAuthProtocol"
			}
			instance = fmt.Sprintf("%s\nauthProtocol: %s", instance, authProtocol)
		}
		if conf.PrivKey != "" {
			instance = fmt.Sprintf("%s\nprivKey: %s", instance, conf.PrivKey)
		}
		if conf.PrivProtocol != "" {
			var privProtocol string
			if conf.PrivProtocol == "DES" {
				privProtocol = "usmDESPrivProtocol"
			} else if conf.PrivProtocol == "AES" {
				privProtocol = "usmAesCfb128Protocol"
			}
			instance = fmt.Sprintf("%s\nprivProtocol: %s", instance, privProtocol)
		}
		if conf.ContextEngineID != "" {
			instance = fmt.Sprintf("%s\ncontext_engine_id: %s", instance, conf.ContextEngineID)
		}
		if conf.ContextName != "" {
			instance = fmt.Sprintf("%s\ncontext_name: %s", instance, conf.ContextName)
		}
		instances := [][]integration.Data{{integration.Data(instance)}}
		initConfigs := [][]integration.Data{{integration.Data("")}}
		newConfigs := buildTemplates(adIdentifier, []string{"snmp"}, initConfigs, instances)
		for i := range newConfigs {
			// Schedule cluster checks when running in k8s
			newConfigs[i].ClusterCheck = config.IsKubernetes()
		}
		allConfigs = append(allConfigs, newConfigs...)
	}
	return allConfigs, nil
}

func init() {
	RegisterProvider(names.SNMP, NewSNMPConfigProvider)
}
