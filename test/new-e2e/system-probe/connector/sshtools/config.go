// Copyright (C) 2017 ScyllaDB

package sshtools

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

// Config specifies SSH configuration.
type Config struct {
	ssh.ClientConfig `json:"-" yaml:"-"`
	// Port specifies the port number to connect on the remote host.
	Port int `yaml:"port"`
	// ServerAliveInterval specifies an interval to send keepalive message
	// through the encrypted channel and request a response from the server.
	ServerAliveInterval time.Duration `yaml:"server_alive_interval"`
	// ServerAliveCountMax specifies the number of server keepalive messages
	// which may be sent without receiving any messages back from the server.
	// If this threshold is reached while server keepalive messages are being sent,
	// ssh will disconnect from the server, terminating the session.
	ServerAliveCountMax int `yaml:"server_alive_count_max"`
	// Pty specifies if a pty should be associated with sessions on remote
	// hosts. Enabling pty would make Scylla banner to be printed to commands'
	// stdout.
	Pty bool `yaml:"pty"`
}

// DefaultConfig returns a Config initialized with default values.
func DefaultConfig() Config {
	return Config{
		Port:                22,
		ServerAliveInterval: 15 * time.Second,
		ServerAliveCountMax: 3,
	}
}

// Validate checks if all the fields are properly set.
func (c Config) Validate() (err error) {
	if c.Port <= 0 {
		err = errors.Join(err, errors.New("invalid port, must be > 0"))
	}

	if c.ServerAliveInterval < 0 {
		err = errors.Join(err, errors.New("invalid server_alive_interval, must be >= 0"))
	}

	if c.ServerAliveCountMax < 0 {
		err = errors.Join(err, errors.New("invalid server_alive_count_max, must be >= 0"))
	}

	return
}

// WithIdentityFileAuth returns a copy of c with added user and identity file
// authentication method.
func (c Config) WithIdentityFileAuth(user string, identityFile []byte) (Config, error) {
	if user == "" {
		return Config{}, errors.New("missing user")
	}

	auth, err := keyPairAuthMethod(identityFile)
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse identity file: %w", err)
	}

	config := c
	config.User = user
	config.Auth = []ssh.AuthMethod{auth}
	config.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	return config, nil
}

func keyPairAuthMethod(pemBytes []byte) (ssh.AuthMethod, error) {
	signer, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		return nil, err
	}

	return ssh.PublicKeys(signer), nil
}

// KeepaliveEnabled returns true if SSH keepalive should be enabled.
func (c Config) KeepaliveEnabled() bool {
	return c.ServerAliveInterval > 0 && c.ServerAliveCountMax > 0
}
