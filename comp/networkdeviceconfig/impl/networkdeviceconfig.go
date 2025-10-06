// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package networkdeviceconfigimpl implements the networkdeviceconfig component interface
package networkdeviceconfigimpl

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkdeviceconfig "github.com/DataDog/datadog-agent/comp/networkdeviceconfig/def"
	"golang.org/x/crypto/ssh"
	"strings"
)

// Requires defines the dependencies for the networkdeviceconfig component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Logger    log.Component
}

// Provides defines the output of the networkdeviceconfig component
type Provides struct {
	Comp networkdeviceconfig.Component
}

type networkDeviceConfigImpl struct {
	config        *ProcessedNcmConfig
	log           log.Component
	clientFactory RemoteClientFactory
}

// NewComponent creates a new networkdeviceconfig component
func NewComponent(reqs Requires) (Provides, error) {
	ncmConfig, err := newConfig(reqs.Config)
	if err != nil {
		return Provides{}, reqs.Logger.Errorf("Failed to read network device configuration: %v", err)
	}
	impl := &networkDeviceConfigImpl{
		config:        ncmConfig,
		log:           reqs.Logger,
		clientFactory: &SSHClientFactory{},
	}
	provides := Provides{
		Comp: impl,
	}
	return provides, nil
}

// RetrieveRunningConfig retrieves the running configuration for a given network device IP
func (n networkDeviceConfigImpl) RetrieveRunningConfig(ipAddress string) (string, error) {
	commands := []string{
		`show running-config`,
	}
	return n.retrieveConfiguration(ipAddress, commands)
}

// RetrieveStartupConfig retrieves the startup configuration for a given network device IP
func (n networkDeviceConfigImpl) RetrieveStartupConfig(ipAddress string) (string, error) {
	commands := []string{
		`show startup-config`,
	}
	return n.retrieveConfiguration(ipAddress, commands)
}

// retrieveConfiguration retrieves the configuration for a given network device IP
func (n networkDeviceConfigImpl) retrieveConfiguration(ipAddress string, commands []string) (string, error) {
	deviceConfig, ok := n.config.Devices[ipAddress]
	if !ok {
		return "", n.log.Errorf("No authentication credentials found for device %s", ipAddress)
	}
	client, err := n.clientFactory.Connect(ipAddress, deviceConfig.Auth)
	if err != nil {
		return "", n.log.Errorf("Failed to connect to host %s: %v", ipAddress, err)
	}
	defer client.Close()

	result := []string{}

	for _, cmd := range commands {
		session, err := client.NewSession()
		if err != nil {
			return "", n.log.Errorf("Failed to create session to device %s: %s", ipAddress, err)
		}
		n.log.Debugf("Running command: %s\n", cmd)
		output, err := session.CombinedOutput(cmd)
		if err != nil {
			session.Close()
			return "", n.log.Errorf("Command %s on device %s failed: %s\n", cmd, ipAddress, err)
		}
		result = append(result, string(output))
		session.Close()
	}
	return strings.Join(result[:], "\n"), nil
}

// connectToHost establishes an SSH connection to the specified IP address using the provided authentication credentials
func connectToHost(ipAddress string, ac AuthCredentials) (*ssh.Client, error) {
	sshConfig := &ssh.ClientConfig{
		User: ac.Username,
		Auth: []ssh.AuthMethod{ssh.Password(ac.Password)},
	}
	// ⚠️TODO: Use a proper host key callback in production code (pull in known hosts file from user, etc.)
	sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	host := fmt.Sprintf("%s:%s", ipAddress, ac.Port)
	client, err := ssh.Dial(ac.Protocol, host, sshConfig)
	if err != nil {
		return nil, err
	}
	return client, nil
}
