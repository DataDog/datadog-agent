// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package remote

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestSSHClient_RetrieveRunningConfig_Success(t *testing.T) {
	expectedConfig := `
version 15.1
hostname Router1
interface GigabitEthernet0/1
 ip address 192.168.1.1 255.255.255.0
end`

	session := &mockSSHSession{
		outputs: map[string]string{
			"show running-config": expectedConfig,
		},
	}

	client := &MockSSHClient{
		session: session,
	}

	config, err := client.RetrieveRunningConfig()

	assert.NoError(t, err)
	assert.Equal(t, expectedConfig, config)
	assert.True(t, session.closed, "Session should be closed after use")
}

func TestSSHClient_RetrieveStartupConfig_Success(t *testing.T) {
	expectedConfig := `
version 15.1
hostname Router1
interface GigabitEthernet0/1
 ip address 192.168.1.1 255.255.255.0
end`

	session := &mockSSHSession{
		outputs: map[string]string{
			"show startup-config": expectedConfig,
		},
	}

	client := &MockSSHClient{
		session: session,
	}

	config, err := client.RetrieveStartupConfig()

	assert.NoError(t, err)
	assert.Equal(t, expectedConfig, config)
	assert.True(t, session.closed, "Session should be closed after use")
}

func TestSSHClient_RetrieveConfig_SessionCreationFailure(t *testing.T) {
	client := &MockSSHClient{
		sessionError: errors.New("failed to create SSH session"),
	}

	_, err := client.RetrieveRunningConfig()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SSH session")
}

func TestSSHClient_RetrieveConfig_CommandExecutionFailure(t *testing.T) {
	session := &mockSSHSession{
		err: errors.New("command execution failed"),
	}

	client := &MockSSHClient{
		session: session,
	}

	_, err := client.RetrieveRunningConfig()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command execution failed")
	assert.True(t, session.closed, "Session should be closed even on failure")
}

func TestSSHClient_MultipleCommands(t *testing.T) {
	session := &mockSSHSession{
		outputs: map[string]string{
			"show version":    "Cisco IOS Software Version 15.1",
			"show interfaces": "GigabitEthernet0/1 is up, line protocol is up",
			"show ip route":   "Gateway of last resort is not set",
		},
	}

	client := &MockSSHClient{
		session: session,
	}

	commands := []string{"show version", "show interfaces", "show ip route"}
	results, err := client.retrieveConfiguration(commands)

	assert.NoError(t, err)
	assert.Contains(t, results, "Cisco IOS Software Version 15.1")
	assert.Contains(t, results, "GigabitEthernet0/1 is up, line protocol is up")
	assert.Contains(t, results, "Gateway of last resort is not set")
	assert.True(t, session.closed, "Session should be closed after use")
}

func TestBuildHostKeyCallback(t *testing.T) {
	// Create a temporary known_hosts file for testing
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	// Generate a real test key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Convert to SSH public key format
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)

	// Create a properly formatted known_hosts entry
	// Format: hostname keytype base64key
	knownHostsContent := fmt.Sprintf("testhost.example.com %s %s\n",
		publicKey.Type(),
		base64.StdEncoding.EncodeToString(publicKey.Marshal()))

	err = os.WriteFile(knownHostsPath, []byte(knownHostsContent), 0600)
	require.NoError(t, err)

	tests := []struct {
		name   string
		config *ncmconfig.SSHConfig
		logMsg string
		errMsg string
	}{
		{
			name: "success: valid known_hosts path",
			config: &ncmconfig.SSHConfig{
				KnownHostsPath: knownHostsPath,
			},
		},
		{
			name: "error: invalid known_hosts path",
			config: &ncmconfig.SSHConfig{
				KnownHostsPath: "/not_real_path",
			},
			errMsg: "error parsing known_hosts file from path: open /not_real_path",
		},
		{
			name: "success + warn log: use insecure ignore host key",
			config: &ncmconfig.SSHConfig{
				InsecureSkipVerify: true,
			},
			logMsg: "SSH host key verification is disabled - connects are insecure",
		},
		{
			name:   "error: no host key verification configured",
			config: &ncmconfig.SSHConfig{},
			errMsg: "No SSH host key configured: set known_hosts file path or enable insecure_skip_verify",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// initialize capturing logging
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := log.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, log.DebugLvl)
			require.NoError(t, err)
			log.SetupLogger(l, "debug")

			callback, err := buildHostKeyCallback(tt.config)
			if tt.errMsg != "" {
				assert.ErrorContains(t, err, tt.errMsg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, callback)
			}

			if tt.logMsg != "" {
				w.Flush()
				logOutput := b.String()
				assert.Contains(t, logOutput, tt.logMsg)
			}

		})
	}
}

func TestValidateClientConfig(t *testing.T) {
	supportedAlgos := ssh.SupportedAlgorithms()
	validCipher := supportedAlgos.Ciphers[0]
	validKex := supportedAlgos.KeyExchanges[0]
	validHostKey := supportedAlgos.HostKeys[0]

	tests := []struct {
		name        string
		config      *ncmconfig.SSHConfig
		errContains string
	}{
		{
			name: "valid config with all algorithms",
			config: &ncmconfig.SSHConfig{
				Ciphers:           []string{validCipher},
				KeyExchanges:      []string{validKex},
				HostKeyAlgorithms: []string{validHostKey},
			},
		},
		{
			name: "valid config with empty algorithms",
			config: &ncmconfig.SSHConfig{
				Ciphers:           []string{},
				KeyExchanges:      []string{},
				HostKeyAlgorithms: []string{},
			},
		},
		{
			name: "invalid cipher",
			config: &ncmconfig.SSHConfig{
				Ciphers:           []string{"bad-cipher"},
				KeyExchanges:      []string{validKex},
				HostKeyAlgorithms: []string{validHostKey},
			},
			errContains: "unsupported cipher",
		},
		{
			name: "invalid key exchange",
			config: &ncmconfig.SSHConfig{
				Ciphers:           []string{validCipher},
				KeyExchanges:      []string{"bad-kex"},
				HostKeyAlgorithms: []string{validHostKey},
			},
			errContains: "unsupported key exchange",
		},
		{
			name: "invalid host key algorithm",
			config: &ncmconfig.SSHConfig{
				Ciphers:           []string{validCipher},
				KeyExchanges:      []string{validKex},
				HostKeyAlgorithms: []string{"bad-host-key"},
			},
			errContains: "unsupported host key algorithm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateClientConfig(tt.config)

			if tt.errContains != "" {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBuildAuthMethods(t *testing.T) {
	// Create a temporary known_hosts file for testing
	tmpDir := t.TempDir()
	privateKeyPath := filepath.Join(tmpDir, "private_key.pem")
	privateKeyWithPassphrase := filepath.Join(tmpDir, "private_key_passphrase.pem")
	invalidKeyPath := filepath.Join(tmpDir, "invalid_key.pem")

	// private key case w/o passphrase
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(privateKey, "")
	require.NoError(t, err)
	privateKeyPEM := pem.EncodeToMemory(block)
	err = os.WriteFile(privateKeyPath, privateKeyPEM, 0600)
	require.NoError(t, err)

	// private key w/ passphrase
	encrypted, err := ssh.MarshalPrivateKeyWithPassphrase(privateKey, "", []byte("passphrase"))
	require.NoError(t, err)
	pemBlock := pem.EncodeToMemory(encrypted)
	require.NoError(t, err)
	err = os.WriteFile(privateKeyWithPassphrase, pemBlock, 0600)
	require.NoError(t, err)

	// invalid key (invalid content)
	err = os.WriteFile(invalidKeyPath, []byte("not a real key"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name                string
		auth                ncmconfig.AuthCredentials
		expectedAuthMethods int
		errMsg              string
	}{
		{
			name: "success: only passphrase",
			auth: ncmconfig.AuthCredentials{
				Username: "test",
				Password: "hunter2",
			},
			expectedAuthMethods: 1,
		},
		{
			name: "success: private key only",
			auth: ncmconfig.AuthCredentials{
				Username:       "test",
				PrivateKeyFile: privateKeyPath,
			},
			expectedAuthMethods: 1,
		},
		{
			name: "success: private key with passphrase",
			auth: ncmconfig.AuthCredentials{
				Username:             "test",
				PrivateKeyFile:       privateKeyWithPassphrase,
				PrivateKeyPassphrase: "passphrase",
			},
			expectedAuthMethods: 1,
		},
		{
			name: "success: 2 auth methods, user/pass + private key",
			auth: ncmconfig.AuthCredentials{
				Username:       "test",
				Password:       "hunter2",
				PrivateKeyFile: privateKeyPath,
			},
			expectedAuthMethods: 2,
		},
		{
			name: "error: cannot read private key",
			auth: ncmconfig.AuthCredentials{
				PrivateKeyFile: "/not_real_path",
			},
			errMsg: "error reading private key:",
		},
		{
			name: "error: cannot parse private key",
			auth: ncmconfig.AuthCredentials{
				PrivateKeyFile: invalidKeyPath,
			},
			errMsg: "error parsing private key:",
		},
		{
			name:   "error: no auth methods configured",
			errMsg: "no SSH authentication methods configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authMethods, err := buildAuthMethods(tt.auth)
			if tt.errMsg != "" {
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, authMethods)
			} else {
				assert.NoError(t, err)
				assert.Len(t, authMethods, tt.expectedAuthMethods)
				for _, method := range authMethods {
					assert.NotNil(t, method)
				}
			}
		})
	}
}
