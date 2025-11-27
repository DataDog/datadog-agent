// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkdeviceconfigimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

// AuthCredentials holds the authentication credentials to connect to a network device.
type AuthCredentials struct { // auth_credentials
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Port     string `mapstructure:"port"`
	Protocol string `mapstructure:"protocol"`
	// TODO: Uncomment and implement SSH key support
	//SshKeyPath       string `mapstructure:"sshKeyPath"`       // path to the SSH key file
	//SshKeyPassphrase string `mapstructure:"sshKeyPassphrase"` // passphrase for SSH key if needed
	//Enable           bool   `mapstructure:"enable"`           // if true, will use enablePassword to enter privileged exec mode
	//EnablePassword   string `mapstructure:"enable_password"`  // to be able to use privileged exec mode
}

// DeviceConfig holds the info to connect to a network device, including its IP address and authentication credentials.
type DeviceConfig struct {
	IPAddress string          `mapstructure:"ip_address"` // ip address of the network device, e.g., "10.0.0.1"
	Auth      AuthCredentials `mapstructure:"auth"`
}

// RawNcmConfig is the raw config structure for Network Config Management (NCM) taken from the Agent configuration
type RawNcmConfig struct {
	Namespace string         `mapstructure:"namespace"` // namespace for the network config management, e.g., "default"
	Devices   []DeviceConfig `mapstructure:"devices"`
}

// ProcessedNcmConfig is the processed config structure for Network Config Management (NCM) to be used by the component
type ProcessedNcmConfig struct {
	Namespace string
	Devices   map[string]DeviceConfig // map of device IP addresses to DeviceConfig
}

func newConfig(agentConfig config.Component) (*ProcessedNcmConfig, error) {
	ncm := &RawNcmConfig{}
	err := structure.UnmarshalKey(agentConfig, "network_device_config_management", &ncm)
	if err != nil {
		return &ProcessedNcmConfig{}, err
	}
	deviceMap := make(map[string]DeviceConfig)
	for _, d := range ncm.Devices {
		deviceMap[d.IPAddress] = d
	}
	return &ProcessedNcmConfig{
		Namespace: ncm.Namespace,
		Devices:   deviceMap,
	}, nil
}
