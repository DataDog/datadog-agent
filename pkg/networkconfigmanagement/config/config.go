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
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/snmp/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
)

var checkName = "network_config_management"
var defaultCheckInterval = 15 * time.Minute

// AuthCredentials holds the authentication credentials to connect to a network device.
type AuthCredentials struct { // auth_credentials
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Port     string `yaml:"port"`
	Protocol string `yaml:"remote"`
	// SSH-specific configurations
	// TODO: Move to separate struct if needed (e.g., SSHConfig)
	SSHCiphers      []string `yaml:"ssh_ciphers"`
	SSHKeyExchanges []string `yaml:"ssh_key_exchanges"`
	SSHHostKeyAlgos []string `yaml:"ssh_host_key_algorithms"`
	// TODO: Uncomment and implement SSH key support
	//SshKeyPath       string `yaml:"sshKeyPath"`       // path to the SSH key file
	//SshKeyPassphrase string `yaml:"sshKeyPassphrase"` // passphrase for SSH key if needed
	//Enable           bool   `yaml:"enable"`           // if true, will use enablePassword to enter privileged exec mode
	//EnablePassword   string `yaml:"enable_password"`  // to be able to use privileged exec mode
}

// DeviceInstance holds the initial config to connect to a network device, including its IP address and authentication credentials.
type DeviceInstance struct {
	IPAddress string          `yaml:"ip_address"` // ip address of the network device, e.g., "10.0.0.1"
	Auth      AuthCredentials `yaml:"auth"`
}

// InitConfig holds the initial configuration for the NCM component, including the namespace and check interval.
type InitConfig struct {
	Namespace             string `yaml:"namespace"`               // Namespace for the NCM devices where configs are retrieved from, to help match a device on DD
	MinCollectionInterval int    `yaml:"min_collection_interval"` // Interval in seconds to check for config changes
}

// NcmCheckContext holds the processed config needed for an integration instance to run
type NcmCheckContext struct {
	Namespace             string
	Device                *DeviceInstance
	MinCollectionInterval time.Duration
}

// NcmComponentContext is the processed config structure for Network Config Management (NCM) to be used by the component
type NcmComponentContext struct {
	Namespace string
	Devices   map[string]DeviceInstance // map of device IP addresses to DeviceInstance
}

// NewNcmCheckContext creates a new NcmCheckContext from raw instance and init config data
func NewNcmCheckContext(rawInstance integration.Data, rawInitConfig integration.Data) (*NcmCheckContext, error) {
	var err error

	ncc := &NcmCheckContext{}

	var deviceInstance DeviceInstance
	var initConfig InitConfig

	// Parse instance (for device)
	err = yaml.Unmarshal(rawInstance, &deviceInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal device config: %s", err)
	}
	// Validate device instance
	err = deviceInstance.ValidateDeviceInstance()
	if err != nil {
		return nil, fmt.Errorf("invalid device config for device %s: %w", deviceInstance.IPAddress, err)
	}
	// Set device instance in context
	ncc.Device = &deviceInstance

	//Parse init config
	err = yaml.Unmarshal(rawInitConfig, &initConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal init config: %s", err)
	}
	// Validate and apply defaults for init config
	err = initConfig.ValidateInitConfig()
	if err != nil {
		return nil, err
	}
	ncc.Namespace = initConfig.Namespace
	ncc.MinCollectionInterval = time.Duration(initConfig.MinCollectionInterval)

	return ncc, nil
}

// GetNCMContextFromCoreCheck retrieves the NCM configurations from the agent's config for the integration
// TODO: tests for this to come when the component is refactored / we're working on the trigger-based approach for config changes
func GetNCMContextFromCoreCheck(client ipc.HTTPClient) (*NcmComponentContext, error) {
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
	var ncc NcmComponentContext

	// Iterate through the instances (devices) and parse the device configs
	var deviceInstances []DeviceInstance
	for _, c := range cr.Configs {
		if c.Config.Name == checkName { // Check name for NCM
			// Parse each instance / device
			for _, instance := range c.Config.Instances {
				var deviceInstance DeviceInstance
				err := yaml.Unmarshal(instance, &deviceInstance)
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal NCM device config: %s", err)
				}
				err = deviceInstance.ValidateDeviceInstance()
				if err != nil {
					return nil, fmt.Errorf("invalid device config for device %s: %w", deviceInstance.IPAddress, err)
				}
				deviceInstances = append(deviceInstances, deviceInstance)
			}
			// Parse init config if exists
			if c.Config.InitConfig != nil {
				var initConfig InitConfig
				err := yaml.Unmarshal(c.Config.InitConfig, &initConfig)
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal init config: %s", err)
				}
				err = initConfig.ValidateInitConfig()
				if err != nil {
					return nil, err
				}
				ncc.Namespace = initConfig.Namespace
			}
		}
	}
	// Make device map to easily reference from component when retrieving configs upon event of config change
	deviceMap := make(map[string]DeviceInstance)
	for _, d := range deviceInstances {
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
	ncc.Devices = deviceMap

	return &ncc, nil
}

// ValidateInitConfig checks that the InitConfig has all required fields and applies defaults where needed
func (ic *InitConfig) ValidateInitConfig() error {
	// apply default values for anything missing that is not required
	if ic.Namespace == "" {
		log.Debugf("No namespace specified in init config, applying default: %s", "default")
		ic.Namespace = "default"
	}
	namespace, err := utils.NormalizeNamespace(ic.Namespace)
	if err != nil {
		return err
	}
	ic.Namespace = namespace
	if ic.MinCollectionInterval <= 0 {
		log.Debugf("No or invalid min_collection_interval specified in init config, applying default: %d", defaultCheckInterval)
		ic.MinCollectionInterval = int(defaultCheckInterval.Seconds()) // Default to 15 minutes
	}
	return nil
}

// ValidateDeviceInstance checks that the DeviceInstance has all required fields and applies defaults where needed
func (dc *DeviceInstance) ValidateDeviceInstance() error {
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
func (dc *DeviceInstance) applyDefaults() {
	if dc.Auth.Port == "" {
		log.Debugf("Applying default port for device %s: %s", dc.IPAddress, "22")
		dc.Auth.Port = "22"
	}
	if dc.Auth.Protocol == "" {
		log.Debugf("Applying default protocol for device %s: %s", dc.IPAddress, "tcp")
		dc.Auth.Protocol = "tcp"
	}
}
