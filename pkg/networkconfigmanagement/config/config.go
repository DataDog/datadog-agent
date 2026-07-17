// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package config provides configuration structures and functions for the Network Config Management (NCM) core check and component
package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/snmp/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var checkName = "network_config_management"
var defaultCheckInterval = 15 * time.Minute
var defaultSSHTimeout = 30 * time.Second
var defaultInventoryReportMaxInterval = 1 * time.Hour

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
	Namespace string          `yaml:"namespace"`  // namespace for the device; if empty, defaults to value from initconfig
	Profile   string          `yaml:"profile"`    // device profile name, e.g., "cisco-ios"
	Auth      AuthCredentials `yaml:"auth"`
}

// DeviceID returns the formatted ID for this DeviceInstance.
func (di *DeviceInstance) DeviceID() string {
	return fmt.Sprintf("%s:%s", di.Namespace, di.IPAddress)
}

// InitConfig holds the initial configuration for the NCM component, including the namespace and check interval.
type InitConfig struct {
	Namespace                  string        `yaml:"namespace"`                     // Namespace for the NCM devices where configs are retrieved from, to help match a device on DD
	MinCollectionInterval      time.Duration `yaml:"min_collection_interval"`       // Interval in seconds to check for config changes
	InventoryReportMaxInterval time.Duration `yaml:"inventory_report_max_interval"` // Slowest cadence (in seconds) for sending an inventory report; a report is also sent any time a new config is captured
	SSH                        *SSHConfig    `yaml:"ssh"`                           // SSH holds global connection configurations that can apply to all devices if pertinent
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
	Device                     *DeviceInstance
	MinCollectionInterval      time.Duration
	InventoryReportMaxInterval time.Duration
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
	// Apply defaults if missing optional values
	initConfig.applyDefaults()
	if err = initConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid init config: %w", err)
	}

	var deviceInstance DeviceInstance
	err = yaml.Unmarshal(rawInstance, &deviceInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal device config: %s", err)
	}
	deviceInstance.applyDefaults(&initConfig)
	if err = deviceInstance.Validate(); err != nil {
		return nil, fmt.Errorf("invalid device config for %s: %w", deviceInstance.IPAddress, err)
	}

	// Build the final context to send out
	ncc := &NcmCheckContext{
		MinCollectionInterval:      time.Duration(initConfig.MinCollectionInterval) * time.Second,
		InventoryReportMaxInterval: time.Duration(initConfig.InventoryReportMaxInterval) * time.Second,
		Device:                     &deviceInstance,
	}
	return ncc, nil
}

// GetNCMContextFromCoreCheck retrieves the NCM configurations from the agent's config for the integration
// TODO: tests for this to come when the component is refactored / we're working on the trigger-based approach for config changes
func GetNCMContextFromCoreCheck(ctx context.Context, client ipc.HTTPClient, ipAddr string) (*NcmCheckContext, error) {
	// Call the agent's config check endpoint to retrieve the NCM configs (from core check)
	endpoint, err := client.NewIPCEndpoint("/agent/config-check")
	if err != nil {
		return nil, err
	}
	urlValues := url.Values{}
	urlValues.Set("raw", "true")
	res, err := endpoint.DoGet(ipchttp.WithContext(ctx), ipchttp.WithValues(urlValues))
	if err != nil {
		return nil, err
	}
	cr := integration.ConfigCheckResponse{}
	err = json.Unmarshal(res, &cr)
	if err != nil {
		return nil, err
	}

	// Iterate through the instances (devices) and parse the device configs
	for _, c := range cr.Configs {
		if c.Config.Name == checkName { // config is for NCM
			for _, instance := range c.Config.Instances { // find the instance for this IP
				var deviceInstance DeviceInstance
				err := yaml.Unmarshal(instance, &deviceInstance)
				if err != nil {
					return nil, fmt.Errorf("unable to parse NCM config: %v", err)
				}
				if deviceInstance.IPAddress != ipAddr {
					continue
				}
				return NewNcmCheckContext(instance, c.Config.InitConfig)
			}
		}
	}
	return nil, errors.New("no NCM configuration found")
}

func (ic *InitConfig) applyDefaults() {
	if ic.Namespace == "" {
		log.Debugf("No namespace specified in init config, applying default: %s", "default")
		ic.Namespace = "default"
	}
	if ic.MinCollectionInterval <= 0 {
		log.Debugf("No or invalid min_collection_interval specified in init config, applying default: %d", defaultCheckInterval)
		ic.MinCollectionInterval = defaultCheckInterval // Default to 15 minutes
	}
	if ic.InventoryReportMaxInterval <= 0 {
		log.Debugf("No or invalid inventory_report_max_interval specified in init config, applying default: %s", defaultInventoryReportMaxInterval)
		ic.InventoryReportMaxInterval = defaultInventoryReportMaxInterval
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

	if ic.InventoryReportMaxInterval <= 0 {
		return errors.New("inventory_report_max_interval must be greater than 0")
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
func (di *DeviceInstance) applyDefaults(initConfig *InitConfig) {
	if di.Auth.Port == "" {
		log.Debugf("Applying default port for device %s: %s", di.IPAddress, "22")
		di.Auth.Port = "22"
	}
	if di.Auth.Protocol == "" {
		log.Debugf("Applying default protocol for device %s: %s", di.IPAddress, "tcp")
		di.Auth.Protocol = "tcp"
	}
	// Device-specific SSH config takes precedence, if not set, use init_config's SSH config as a "global"
	if di.Auth.SSH == nil && initConfig != nil {
		di.Auth.SSH = initConfig.SSH
	}
	if di.Namespace == "" && initConfig != nil {
		di.Namespace = initConfig.Namespace
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
	if di.Auth.SSH == nil {
		return fmt.Errorf(authBaseString, "SSH configuration", di.IPAddress)
	}

	return nil
}

func (sc *SSHConfig) validate() error {
	// apply defaults that are recommended
	if sc.Timeout <= 0 {
		log.Debugf("no or invalid SSH timeout specified in config, applying default: %d", defaultSSHTimeout)
		sc.Timeout = defaultSSHTimeout
	} else if sc.Timeout < time.Millisecond {
		// bug - we released this as time.Duration, which when customers inserted values as integers like "30" for 30 seconds,
		// it was being interpreted as 30 nanoseconds which caused connection timeout issues. Some customers migrated
		// to duration formats like "30s" - but we generally don't want this to be the standard behavior or to document this.
		// To maintain backwards compatibility, we will apply a transform for any values that are less than
		// 1 Millisecond (1,000,000 nanoseconds) to be interpreted as seconds and convert them to the appropriate duration value.
		log.Debugf("SSH timeout value %d is too low, applying transform to interpret as seconds: %d", sc.Timeout, sc.Timeout*time.Second)
		sc.Timeout = sc.Timeout * time.Second
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
