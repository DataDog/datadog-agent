// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package config provides configuration structures and functions for the Network Config Management (NCM) core check and component
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/snmp/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.yaml.in/yaml/v2"
)

var checkName = "network_config_management"
var defaultCheckInterval = 15 * time.Minute
var defaultSSHTimeout = 30 * time.Second

// AuthCredentials holds the authentication credentials to connect to a network device.
type AuthCredentials struct { // auth_credentials
	// Authenticate via password (fallback after private key if provided)
	Username string `yaml:"username"`
	Password string `yaml:"password"`

	// Authenticate via private key and/or passphrase
	PrivateKeyFile       string `yaml:"private_key_file"`
	PrivateKeyPassphrase string `yaml:"private_key_passphrase"`

	Port     string `yaml:"port"`
	Protocol string `yaml:"remote"`

	SSH *SSHConfig `yaml:"ssh"`
}

// DeviceInstance holds the initial config to connect to a network device, including its IP address and authentication credentials.
type DeviceInstance struct {
	IPAddress string          `yaml:"ip_address"` // ip address of the network device, e.g., "10.0.0.1"
	Profile   string          `yaml:"profile"`    // device profile name, e.g., "cisco-ios"
	Auth      AuthCredentials `yaml:"auth"`
}

// InitConfig holds the initial configuration for the NCM component, including the namespace and check interval.
type InitConfig struct {
	Namespace             string     `yaml:"namespace"`               // Namespace for the NCM devices where configs are retrieved from, to help match a device on DD
	MinCollectionInterval int        `yaml:"min_collection_interval"` // Interval in seconds to check for config changes
	SSH                   *SSHConfig `yaml:"ssh"`                     // SSH holds global connection configurations that can apply to all devices if pertinent
}

// SSHConfig holds the configuration (either globally if in init config or for the specific device instance) to use when connecting to the configured device via SSH
type SSHConfig struct {
	// General configurations for SSH connections
	Timeout time.Duration `yaml:"timeout"` // Timeout specifies max amount of time for the SSH client to allow the TCP connection to establish

	// For host key verification (verify identity of remote server/host)
	KnownHostsPath     string `yaml:"known_hosts_path"`     // KnownHostsPath is the location that contains public keys to servers and verify identity of servers we connect to
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"` // InsecureSkipVerify is a boolean for development/testing purposes to skip host key validation (insecure)

	// SSH-specific encryption algorithms to use with a device for establishing/securing a connection
	Ciphers           []string `yaml:"ciphers"`
	KeyExchanges      []string `yaml:"key_exchanges"`
	HostKeyAlgorithms []string `yaml:"host_key_algorithms"`
	// Allow weak/legacy algorithms (from above) be used for older devices that do not support recommended algorithms (insecure)
	AllowLegacyAlgorithms bool `yaml:"allow_legacy_algorithms"`
}

// NcmCheckContext holds the processed config needed for an integration instance to run
type NcmCheckContext struct {
	Namespace             string
	Device                *DeviceInstance
	MinCollectionInterval time.Duration
	ProfileMap            profile.Map
	ProfileCache          *profile.Cache
}

// NcmComponentContext is the processed config structure for Network Config Management (NCM) to be used by the component
type NcmComponentContext struct {
	Namespace string
	Devices   map[string]DeviceInstance // map of device IP addresses to DeviceInstance
}

// NewNcmCheckContext creates a new NcmCheckContext from raw instance and init config data
func NewNcmCheckContext(rawInstance integration.Data, rawInitConfig integration.Data) (*NcmCheckContext, error) {
	var err error

	// Unmarshal init config + device instance config
	var initConfig InitConfig
	err = yaml.Unmarshal(rawInitConfig, &initConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal init config: %s", err)
	}
	var deviceInstance DeviceInstance
	err = yaml.Unmarshal(rawInstance, &deviceInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal device config: %s", err)
	}

	// Apply defaults if missing optional values
	initConfig.applyDefaults()
	deviceInstance.applyDefaults()

	// Device-specific SSH config takes precedence, if not set, use init_config's SSH config as a "global"
	if deviceInstance.Auth.SSH == nil {
		deviceInstance.Auth.SSH = initConfig.SSH
	}

	// If still empty (init_config also has no SSH configs), error for needed configuration
	if deviceInstance.Auth.SSH == nil {
		return nil, fmt.Errorf("no SSH configuration found in device instance or init_config or device %s", deviceInstance.IPAddress)
	}

	// Validate configs after all of that
	if err = initConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid init config: %w", err)
	}
	if err = deviceInstance.Validate(); err != nil {
		return nil, fmt.Errorf("invalid device config for %s: %w", deviceInstance.IPAddress, err)
	}

	// Populate the profiles map (from defaults/OOTB)
	profMap, err := profile.GetProfileMap("default_profiles")
	if err != nil {
		return nil, fmt.Errorf("failed to get profile map: %w", err)
	}

	profileCache := &profile.Cache{}

	// Build the final context to send out
	ncc := &NcmCheckContext{
		Namespace:             initConfig.Namespace,
		MinCollectionInterval: time.Duration(initConfig.MinCollectionInterval) * time.Second,
		Device:                &deviceInstance,
		ProfileMap:            profMap,
		ProfileCache:          profileCache,
	}
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
				err = deviceInstance.Validate()
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
				err = initConfig.Validate()
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

func (ic *InitConfig) applyDefaults() {
	if ic.Namespace == "" {
		log.Debugf("No namespace specified in init config, applying default: %s", "default")
		ic.Namespace = "default"
	}
	if ic.MinCollectionInterval <= 0 {
		log.Debugf("No or invalid min_collection_interval specified in init config, applying default: %d", defaultCheckInterval)
		ic.MinCollectionInterval = int(defaultCheckInterval.Seconds()) // Default to 15 minutes
	}
}

// Validate checks that the InitConfig has all required fields and applies defaults where needed
func (ic *InitConfig) Validate() error {
	namespace, err := utils.NormalizeNamespace(ic.Namespace)
	if err != nil {
		return err
	}
	ic.Namespace = namespace

	if ic.MinCollectionInterval <= 0 {
		return errors.New("min_collection_interval must be greater than zero")
	}

	// if SSH configs exist, ensure they're valid
	if ic.SSH != nil {
		if err := ic.SSH.validate(); err != nil {
			return fmt.Errorf("invalid init_config SSH config: %w", err)
		}
	}
	return nil
}

// Validate checks that the DeviceInstance has all required fields and applies defaults where needed
func (di *DeviceInstance) Validate() error {
	// check for missing fields that are required
	if err := di.hasRequiredFields(); err != nil {
		return err
	}

	// check for validity of required/optional fields if present
	// TODO: Protocol/network check? Are customers aware of what's possible?
	ip := net.ParseIP(di.IPAddress)
	if ip == nil {
		return fmt.Errorf("invalid ip_address format: %s", di.IPAddress)
	}

	// Port validation
	port, err := strconv.Atoi(di.Auth.Port)
	if err != nil {
		return fmt.Errorf("invalid port, not valid integer: %s", di.Auth.Port)
	}
	if !(port >= 0 && port <= 65535) { // max value for 16-bit unsigned int
		return fmt.Errorf("invalid port, out of range: %s", di.Auth.Port)
	}

	// if SSH configs exist, ensure they are valid
	if di.Auth.SSH != nil {
		if err := di.Auth.SSH.validate(); err != nil {
			return fmt.Errorf("invalid SSH config for device %s: %w", di.IPAddress, err)
		}
	}
	return nil
}

// applyDefaults set default values for any optional fields that are not set + not required
func (di *DeviceInstance) applyDefaults() {
	if di.Auth.Port == "" {
		log.Debugf("Applying default port for device %s: %s", di.IPAddress, "22")
		di.Auth.Port = "22"
	}
	if di.Auth.Protocol == "" {
		log.Debugf("Applying default protocol for device %s: %s", di.IPAddress, "tcp")
		di.Auth.Protocol = "tcp"
	}
}

func (di *DeviceInstance) hasRequiredFields() error {
	// check for missing fields that are required for a device instance
	if di.IPAddress == "" {
		return errors.New("ip_address is required")
	}
	authBaseString := "auth is required: missing %s for device %s"
	if di.Auth.Username == "" {
		return fmt.Errorf(authBaseString, "username", di.IPAddress)
	}
	// must have at least 1 auth method: password or private key
	if di.Auth.Password == "" && di.Auth.PrivateKeyFile == "" {
		return fmt.Errorf(authBaseString, "auth method (either password or private key)", di.IPAddress)
	}

	return nil
}

func (sc *SSHConfig) validate() error {
	// apply defaults that are recommended
	if sc.Timeout <= 0 {
		log.Debugf("no or invalid SSH timeout specified in config, applying default: %d", defaultSSHTimeout)
		sc.Timeout = defaultSSHTimeout
	}
	if err := sc.hasRequiredFields(); err != nil {
		return err
	}
	return nil
}

func (sc *SSHConfig) hasRequiredFields() error {
	// must have at least a known paths specified or skip verification (insecure, only for development/testing purposes)
	if sc.KnownHostsPath == "" && !sc.InsecureSkipVerify {
		return errors.New("no SSH host key verification configured: set known_hosts_path or enable insecure_skip_verify")
	}
	return nil
}
