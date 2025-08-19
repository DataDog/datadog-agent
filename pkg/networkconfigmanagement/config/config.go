// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package config provides configuration structures and functions for the Network Config Management (NCM) core check and component
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
)

// AuthCredentials holds the authentication credentials to connect to a network device.
type AuthCredentials struct { // auth_credentials
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Port     string `yaml:"port"`
	Protocol string `yaml:"remote"`
	// SSH-specific configurations
	// TODO: Move to separate struct if needed (e.g., SSHConfig)
	SSHCiphers      []string `yaml:"ssh_ciphers,omitempty"`
	SSHKeyExchanges []string `yaml:"ssh_key_exchanges,omitempty"`
	SSHHostKeyAlgos []string `yaml:"ssh_host_key_algorithms,omitempty"`
	// TODO: Uncomment and implement SSH key support
	//SshKeyPath       string `yaml:"sshKeyPath"`       // path to the SSH key file
	//SshKeyPassphrase string `yaml:"sshKeyPassphrase"` // passphrase for SSH key if needed
	//Enable           bool   `yaml:"enable"`           // if true, will use enablePassword to enter privileged exec mode
	//EnablePassword   string `yaml:"enable_password"`  // to be able to use privileged exec mode
}

// DeviceConfig holds the info to connect to a network device, including its IP address and authentication credentials.
type DeviceConfig struct {
	Namespace            string          `yaml:"namespace"`
	IPAddress            string          `yaml:"ip_address"`                       // ip address of the network device, e.g., "10.0.0.1"
	CollectStartupConfig bool            `yaml:"collect_startup_config,omitempty"` // whether to collect the startup config (default: false)
	Auth                 AuthCredentials `yaml:"auth"`
}

// NcmConfig is the processed config structure for Network Config Management (NCM) to be used by the component
type NcmConfig struct {
	Namespace string
	Devices   map[string]DeviceConfig // map of device IP addresses to DeviceConfig
}

// InitConfig holds the initial configuration for the NCM component, including the namespace and check interval.
type InitConfig struct {
	Namespace     string `yaml:"namespace"`      // Namespace for the NCM devices where configs are retrieved from, to help match a device on DD
	CheckInterval int    `yaml:"check_interval"` // Interval in seconds to check for config changes
}

// GetNCMConfigsFromAgent retrieves the NCM configurations from the agent's config check endpoint (from core check)
// TODO: tests for this to come when the component is refactored / we're working on the trigger-based approach for config changes
func GetNCMConfigsFromAgent(client ipc.HTTPClient) (*NcmConfig, error) {
	// Call the agent's config check endpoint to retrieve the NCM configs (from core check)
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
	// Iterate through the instances (devices) and parse the device configs
	var deviceConfigs []DeviceConfig
	for _, c := range cr.Configs {
		if c.Config.Name == "network_config_management" { // Check name for NCM
			// Parse each instance / device
			for _, instance := range c.Config.Instances {
				var deviceConfig DeviceConfig
				err := yaml.Unmarshal(instance, &deviceConfig)
				// TODO: validate the device config + defaults?
				if err == nil {
					deviceConfigs = append(deviceConfigs, deviceConfig)
				}
			}
		}
	}
	// Make device map to easily reference from component when retrieving configs upon event of config change
	deviceMap := make(map[string]DeviceConfig)
	for _, d := range deviceConfigs {
		_, exists := deviceMap[d.IPAddress]
		if exists {
			log.Warnf("Duplicate device IP address found in config: %s, skipping duplicate", d.IPAddress)
			continue
		}
		if d.IPAddress != "" {
			deviceMap[d.IPAddress] = d
		} else {
			log.Warnf("Device config missing IP address, skipping: %+v", d)
		}
	}
	// TODO: deal with namespace + defaults?
	return &NcmConfig{
		Namespace: "default", // Default namespace if not specified
		Devices:   deviceMap,
	}, nil
}

// ValidateConfig checks that the DeviceConfig has all required fields and applies defaults where needed
func (dc *DeviceConfig) ValidateConfig() error {
	// apply default values for anything missing that is not required
	dc.applyDefaults()

	// check for missing fields that are required
	if dc.IPAddress == "" {
		return fmt.Errorf("ip_address is required")
	}
	authBaseString := "auth is required: missing %s for device %s"
	if dc.Auth.Username == "" {
		return fmt.Errorf(authBaseString, "username", dc.IPAddress)
	}
	if dc.Auth.Password == "" {
		return fmt.Errorf(authBaseString, "password", dc.IPAddress)
	}

	// TODO: Protocol/network check? Are customers aware of what's possible?
	ip := net.ParseIP(dc.IPAddress)
	if ip == nil {
		return fmt.Errorf("invalid ip_address format: %s", dc.IPAddress)
	}
	// Port validation
	port, err := strconv.Atoi(dc.Auth.Port)
	if err != nil {
		return fmt.Errorf("invalid port, not valid integer: %s", dc.Auth.Port)
	}
	if !(port >= 0 && port <= 65535) { // max value for 16-bit unsigned int
		return fmt.Errorf("invalid port, out of range: %s", dc.Auth.Port)
	}
	return nil
}

// applyDefaults set default values for any optional fields that are not set + not required
func (dc *DeviceConfig) applyDefaults() {
	if dc.Auth.Port == "" {
		log.Debugf("Applying default port for device %s: %s", dc.IPAddress, "22")
		dc.Auth.Port = "22"
	}
	if dc.Auth.Protocol == "" {
		log.Debugf("Applying default protocol for device %s: %s", dc.IPAddress, "tcp")
		dc.Auth.Protocol = "tcp"
	}
}
