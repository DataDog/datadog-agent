// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"

	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	knownCiphers      []string
	knownKeyExchanges []string
	knownHostKeys     []string
)

func init() {
	supported := ssh.SupportedAlgorithms()
	insecure := ssh.InsecureAlgorithms()

	knownCiphers = slices.Concat(supported.Ciphers, insecure.Ciphers)
	knownKeyExchanges = slices.Concat(supported.KeyExchanges, insecure.KeyExchanges)
	knownHostKeys = slices.Concat(supported.HostKeys, insecure.HostKeys)
}

// SSHConnector implements Client using SSH
type SSHConnector struct {
	device *ncmconfig.DeviceInstance // Device configuration for authentication
}

var _ Connector = (*SSHConnector)(nil)

// SSHConnection implements Connection over SSH
type SSHConnection struct {
	client *RetryingSSHClient
	device *ncmconfig.DeviceInstance
	prof   *profile.NCMProfile
}

var _ Connection = (*SSHConnection)(nil)

// NewSSHConnector creates a new SSH connector for the given device configuration
func NewSSHConnector(device *ncmconfig.DeviceInstance) (Connector, error) {
	if device.Auth.SSH != nil {
		if err := ValidateSSHConfig(device.Auth.SSH); err != nil {
			return nil, fmt.Errorf("error validating ssh client config: %w", err)
		}
	} else {
		return nil, errors.New("missing ssh client config")
	}
	return &SSHConnector{
		device: device,
	}, nil
}

func ConnectOverSSH(device *ncmconfig.DeviceInstance) (Connection, error) {
	c, err := NewSSHConnector(device)
	if err != nil {
		return nil, err
	}
	return c.Connect()
}

func buildHostKeyCallback(config *ncmconfig.SSHConfig) (ssh.HostKeyCallback, error) {
	if config.KnownHostsPath != "" {
		callbackFn, err := knownhosts.New(config.KnownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("error parsing known_hosts file from path: %w", err)
		}
		return callbackFn, nil
	}
	if config.InsecureSkipVerify {
		log.Warnf("SSH host key verification is disabled - connects are insecure!")
		// no-dd-sa:go-security/ssh-ignore-keys
		return ssh.InsecureIgnoreHostKey(), nil
	}
	return nil, errors.New("No SSH host key configured: set known_hosts file path or enable insecure_skip_verify")
}

func buildAuthMethods(auth ncmconfig.AuthCredentials) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if auth.PrivateKeyFile != "" {
		key, err := os.ReadFile(auth.PrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("error reading private key: %s", err)
		}

		var signer ssh.Signer
		if auth.PrivateKeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(auth.PrivateKeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return nil, fmt.Errorf("error parsing private key: %s", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if auth.Password != "" {
		methods = append(methods, ssh.Password(auth.Password), ssh.KeyboardInteractive(func(_, _ string, _ []string, echos []bool) ([]string, error) {
			var answers []string
			for _, echo := range echos {
				// simple heuristic: if a prompt has echo=false, then it's probably a password.
				if echo {
					answers = append(answers, "")
				} else {
					answers = append(answers, auth.Password)
				}
			}
			return answers, nil
		}))
	}

	if len(methods) == 0 {
		return nil, errors.New("no SSH authentication methods configured")
	}

	return methods, nil
}

func ValidateSSHConfig(config *ncmconfig.SSHConfig) error {
	var validCiphers, validKeyExchanges, validHostKeys []string
	if config.AllowLegacyAlgorithms {
		// Log a warning about the insecure nature of algorithms, still check that it's a "valid" algorithm vs. only a safe/supported algo
		log.Warnf("checking supported SSH algorithms is disabled - this is insecure and should only be used with legacy devices in controlled environments ")
		validCiphers, validKeyExchanges, validHostKeys = knownCiphers, knownKeyExchanges, knownHostKeys
	} else {
		supported := ssh.SupportedAlgorithms()
		validCiphers, validKeyExchanges, validHostKeys = supported.Ciphers, supported.KeyExchanges, supported.HostKeys
	}
	if err := validateSupportedAlgorithms("cipher", config.Ciphers, validCiphers); err != nil {
		return err
	}
	if err := validateSupportedAlgorithms("key exchange", config.KeyExchanges, validKeyExchanges); err != nil {
		return err
	}
	if err := validateSupportedAlgorithms("host key algorithm", config.HostKeyAlgorithms, validHostKeys); err != nil {
		return err
	}
	return nil
}

func validateSupportedAlgorithms(algoType string, configuredAlgos []string, supportedAlgos []string) error {
	for _, algo := range configuredAlgos {
		if !slices.Contains(supportedAlgos, algo) {
			return fmt.Errorf("unsupported %s: %s", algoType, algo)
		}
	}
	return nil
}

// SetProfile sets the NCM profile that tells the connection what commands to run.
func (c *SSHConnection) SetProfile(profile *profile.NCMProfile) {
	c.prof = profile
}

// Connect establishes a new SSH connection to the specified IP address using the provided authentication credentials
func (c *SSHConnector) Connect() (Connection, error) {
	client, err := NewRetryingSSHClient(func() (*ssh.Client, error) {
		return connectToDevice(c.device)
	})
	if err != nil {
		return nil, err
	}
	return &SSHConnection{
		client: client,
		device: c.device,
	}, nil
}

func (c *SSHConnection) PushConfig(ctx context.Context, rawConfig string) error {
	if c.prof == nil {
		return fmt.Errorf("no device type provided for %q", c.device.IPAddress)
	}
	if len(c.prof.Commands.PushConfig) == 0 {
		return fmt.Errorf("no push commands for profile %q", c.prof.Name)
	}
	for _, untypedCmd := range c.prof.Commands.PushConfig {
		switch cmd := untypedCmd.(type) {
		case *profile.SCPCommand:
			if _, err := ExecuteSCP(ctx, c.client, cmd, rawConfig); err != nil {
				return fmt.Errorf("unable to copy config to device %q: %w", c.device.IPAddress, err)
			}
		case *profile.PlainCommand:
			if _, err := ExecuteCommand(ctx, c.client, cmd); err != nil {
				return fmt.Errorf("error while pushing config to device %q: %w", c.device.IPAddress, err)
			}
		}
	}
	return nil
}

// Verify validates that the profile works as we expect it to
func (c *SSHConnection) Verify(ctx context.Context) error {
	if c.prof == nil {
		return fmt.Errorf("no device type provided for %q", c.device.IPAddress)
	}
	cmd := c.prof.Commands.Verify
	if cmd == nil {
		return fmt.Errorf("no verify command for profile %q", c.prof.Name)
	}
	_, err := c.execute(ctx, cmd)
	return err
}

// RetrieveRunningConfig retrieves the running configuration for the device connected via SSH
func (c *SSHConnection) RetrieveRunningConfig(ctx context.Context) ([]byte, error) {
	if c.prof == nil {
		return nil, fmt.Errorf("no device type provided for %q", c.device.IPAddress)
	}
	cmd := c.prof.Commands.GetRunning
	if cmd == nil {
		return nil, fmt.Errorf("no get_running command for profile %q", c.prof.Name)
	}
	return c.execute(ctx, cmd)
}

// RetrieveStartupConfig retrieves the startup configuration for the device connected via SSH
func (c *SSHConnection) RetrieveStartupConfig(ctx context.Context) ([]byte, error) {
	if c.prof == nil {
		return nil, fmt.Errorf("no device type provided for %q", c.device.IPAddress)
	}
	cmd := c.prof.Commands.GetStartup
	if cmd == nil {
		return nil, fmt.Errorf("no get_startup command for profile %q", c.prof.Name)
	}
	return c.execute(ctx, cmd)
}

func (c *SSHConnection) execute(ctx context.Context, cmd *profile.PlainCommand) ([]byte, error) {
	result, err := ExecuteCommand(ctx, c.client, cmd)
	if err != nil {
		return nil, err
	}
	return []byte(result), nil
}

// Close closes the SSH client connection
func (c *SSHConnection) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// connectToDevice is a shorthand for connectToHost with parameters from the device
func connectToDevice(device *ncmconfig.DeviceInstance) (*ssh.Client, error) {
	return connectToHost(device.IPAddress, device.Auth, device.Auth.SSH)
}

// connectToHost establishes an SSH connection to the specified IP address using the provided authentication credentials
func connectToHost(ipAddress string, auth ncmconfig.AuthCredentials, config *ncmconfig.SSHConfig) (*ssh.Client, error) {
	if config == nil {
		return nil, fmt.Errorf("SSH configuration is required (host verification) but not provided for device %s", ipAddress)
	}
	callback, err := buildHostKeyCallback(config)
	if err != nil {
		return nil, err
	}
	methods, err := buildAuthMethods(auth)
	if err != nil {
		return nil, err
	}
	sshConfig := &ssh.ClientConfig{
		User:            auth.Username,
		Auth:            methods,
		HostKeyCallback: callback,
		Timeout:         config.Timeout,
		Config: ssh.Config{
			Ciphers:      config.Ciphers,
			KeyExchanges: config.KeyExchanges,
		},
		HostKeyAlgorithms: config.HostKeyAlgorithms,
	}

	host := fmt.Sprintf("%s:%s", ipAddress, auth.Port)
	client, err := ssh.Dial(auth.Protocol, host, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", host, err)
	}

	return client, nil
}
