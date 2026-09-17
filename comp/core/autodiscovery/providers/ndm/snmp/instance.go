// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"fmt"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/credentials"
)

// keyConfig is the value of the "snmp" key of the NDM Remote Configuration
// document. The JSON names are the backend contract and must not be renamed.
type keyConfig struct {
	InitConfig initConfig         `json:"init_config"`
	Instances  []documentInstance `json:"instances"`
}

type initConfig struct {
	Loader string     `json:"loader"`
	Ping   pingConfig `json:"ping"`
}

type pingConfig struct {
	Enabled bool `json:"enabled"`
}

// documentInstance carries no credential values: the Agent resolves CredID
// against its own configured credentials.
type documentInstance struct {
	IPAddress string `json:"ip_address"`
	CredID    string `json:"cred_id"`
}

// checkInitConfig is the init config handed to the snmp check. The yaml names
// come from pkg/collector/corechecks/snmp/internal/checkconfig.
type checkInitConfig struct {
	Loader string         `yaml:"loader,omitempty"`
	Ping   checkPingBlock `yaml:"ping"`
}

type checkPingBlock struct {
	Enabled bool `yaml:"enabled"`
}

// checkInstance is one instance handed to the snmp check. port, timeout and
// retries are absent so the check applies its own defaults.
type checkInstance struct {
	IPAddress       string `yaml:"ip_address"`
	SNMPVersion     string `yaml:"snmp_version"`
	CommunityString string `yaml:"community_string,omitempty"`
	User            string `yaml:"user,omitempty"`
	AuthProtocol    string `yaml:"authProtocol,omitempty"`
	AuthKey         string `yaml:"authKey,omitempty"`
	PrivProtocol    string `yaml:"privProtocol,omitempty"`
	PrivKey         string `yaml:"privKey,omitempty"`
	ContextName     string `yaml:"context_name,omitempty"`
}

// renderInitConfig turns the document's init_config into the check's.
func renderInitConfig(ic initConfig) (integration.Data, error) {
	body, err := yaml.Marshal(checkInitConfig{
		Loader: ic.Loader,
		Ping:   checkPingBlock{Enabled: ic.Ping.Enabled},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render the init config: %w", err)
	}
	return integration.Data(body), nil
}

// renderInstance joins an address with its resolved credential into one check
// instance.
func renderInstance(ipAddress string, c credentials.Credential) (integration.Data, error) {
	body, err := yaml.Marshal(checkInstance{
		IPAddress:       ipAddress,
		SNMPVersion:     c.SNMPVersion,
		CommunityString: c.CommunityString,
		User:            c.User,
		AuthProtocol:    c.AuthProtocol,
		AuthKey:         c.AuthKey,
		PrivProtocol:    c.PrivProtocol,
		PrivKey:         c.PrivKey,
		ContextName:     c.ContextName,
	})
	if err != nil {
		// The marshalled body can hold a credential value, so it stays out of the error.
		return nil, fmt.Errorf("failed to render the instance for %s", ipAddress)
	}
	return integration.Data(body), nil
}
