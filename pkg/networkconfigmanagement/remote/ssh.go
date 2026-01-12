// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

package remote

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

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

// SSHClient implements Client using SSH
type SSHClient struct {
	client *ssh.Client
	device *ncmconfig.DeviceInstance // Device configuration for authentication
	prof   *profile.NCMProfile
}

// SSHSession implements Session using an SSH session
type SSHSession struct {
	session *ssh.Session
}

// NewSSHClient creates a new SSH client for the given device configuration
func NewSSHClient(device *ncmconfig.DeviceInstance) (*SSHClient, error) {
	if device.Auth.SSH != nil {
		if err := validateClientConfig(device.Auth.SSH); err != nil {
			return nil, fmt.Errorf("error validating ssh client config: %w", err)
		}
	}

	return &SSHClient{
		device: device,
	}, nil
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
		methods = append(methods, ssh.Password(auth.Password))
	}

	if len(methods) == 0 {
		return nil, errors.New("no SSH authentication methods configured")
	}

	return methods, nil
}

func validateClientConfig(config *ncmconfig.SSHConfig) error {
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

// SetProfile sets the NCM profile for the device for the client to know which commands to be able to run
func (c *SSHClient) SetProfile(profile *profile.NCMProfile) {
	c.prof = profile
}

// redial attempts to re-establish the SSH connection to the device
func (c *SSHClient) redial() error {
	if c.client != nil {
		_ = c.client.Close()
	}
	newClient, err := connectToHost(c.device.IPAddress, c.device.Auth, c.device.Auth.SSH)
	if err != nil {
		return err
	}
	c.client = newClient
	return nil
}

// Connect establishes a new SSH connection to the specified IP address using the provided authentication credentials
func (c *SSHClient) Connect() error {
	client, err := connectToHost(c.device.IPAddress, c.device.Auth, c.device.Auth.SSH)
	if err != nil {
		return err
	}
	c.client = client
	return nil
}

// NewSession creates a new SSH session for the client (needed for every command execution)
func (c *SSHClient) NewSession() (Session, error) {
	sess, err := c.client.NewSession()
	if err != nil && isTransientSSH(err) {
		if rerr := c.redial(); rerr == nil {
			sess, err = c.client.NewSession()
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}
	return &SSHSession{session: sess}, nil
}

// isTransientSSH checks if the error is transient and can be retried (devices that may only accept a limited number of connections)
func isTransientSSH(err error) bool {
	if err == io.EOF {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "unexpected packet in response to channel open") ||
		strings.Contains(s, "channel open") ||
		strings.Contains(s, "connection reset by peer")
}

// CombinedOutput runs a command using the SSH session and returns its output
func (s *SSHSession) CombinedOutput(cmd string) ([]byte, error) {
	if s.session == nil {
		return nil, errors.New("SSH session is nil")
	}
	return s.session.CombinedOutput(cmd)
}

// RetrieveRunningConfig retrieves the running configuration for the device connected via SSH
func (c *SSHClient) RetrieveRunningConfig() ([]byte, error) {
	commands, err := c.prof.GetCommandValues(profile.Running)
	if err != nil {
		return []byte{}, err
	}
	config, err := c.retrieveConfiguration(commands)
	if err != nil {
		return []byte{}, err
	}
	err = c.prof.ValidateOutput(profile.Running, config)
	if err != nil {
		return []byte{}, err
	}
	return config, err
}

// RetrieveStartupConfig retrieves the startup configuration for the device connected via SSH
func (c *SSHClient) RetrieveStartupConfig() ([]byte, error) {
	commands, err := c.prof.GetCommandValues(profile.Startup)
	if err != nil {
		return []byte{}, err
	}
	config, err := c.retrieveConfiguration(commands)
	if err != nil {
		return []byte{}, err
	}
	err = c.prof.ValidateOutput(profile.Startup, config)
	if err != nil {
		return []byte{}, err
	}
	return c.retrieveConfiguration(commands)
}

// retrieveConfiguration retrieves the configuration for a given network device using multiple commands
func (c *SSHClient) retrieveConfiguration(commands []string) ([]byte, error) {
	var result []byte

	for _, cmd := range commands {
		session, err := c.NewSession()
		if err != nil {
			return []byte{}, fmt.Errorf("failed to create session for command %s: %w", cmd, err)
		}

		log.Debugf("Executing command: %s", cmd)
		output, err := session.CombinedOutput(cmd)
		session.Close()

		if err != nil {
			return []byte{}, fmt.Errorf("command %s failed: %w", cmd, err)
		}

		result = append(result, output...)
		result = append(result, '\n')
	}

	return result, nil
}

// Close closes the SSH client connection
func (c *SSHClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Close closes the SSH session
func (s *SSHSession) Close() error {
	return s.session.Close()
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
