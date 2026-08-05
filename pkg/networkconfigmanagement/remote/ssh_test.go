// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remote

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func makeDevice(t testing.TB, srv *FakeSSHServer) *ncmconfig.DeviceInstance {
	t.Helper()
	knownHosts := MakeKnownHostsFile(t, srv)
	port, err := strconv.Atoi(srv.Port())
	if err != nil {
		t.Fatalf("Initializing fake device: %v", err)
	}
	return &ncmconfig.DeviceInstance{
		IPAddress: srv.Host(),
		Auth: ncmconfig.AuthCredentials{
			Username: srv.User(),
			Password: srv.Password(),
			Port:     strconv.Itoa(port),
			Protocol: "tcp",
			SSH: &ncmconfig.SSHConfig{
				KnownHostsPath: knownHosts,
			},
		},
	}
}

func TestSSHConnector(t *testing.T) {
	expectedConfig := `
version 15.1
hostname Router1
interface GigabitEthernet0/1
 ip address 192.168.1.1 255.255.255.0
`
	srv := StartFakeSSHServer(t, map[string]FakeResponse{
		"show running-config": Ok(expectedConfig),
		"show startup-config": Ok(expectedConfig),
	})
	device := makeDevice(t, srv)
	client, err := NewSSHConnector(device)
	require.NoError(t, err)
	conn, err := client.Connect()
	require.NoError(t, err)
	t.Run("no_profile", func(t *testing.T) {
		_, err := conn.RetrieveRunningConfig(context.Background())
		assert.Error(t, err)
		_, err = conn.RetrieveStartupConfig(context.Background())
		assert.Error(t, err)
	})
	conn.SetProfile(&profile.NCMProfile{
		Name:     "test-profile",
		Commands: profile.CommandSet{},
	})
	t.Run("no_command", func(t *testing.T) {
		_, err := conn.RetrieveRunningConfig(context.Background())
		assert.Error(t, err)
		_, err = conn.RetrieveStartupConfig(context.Background())
		assert.Error(t, err)
	})

	conn.SetProfile(&profile.NCMProfile{
		Name: "test-profile",
		Commands: profile.CommandSet{
			GetRunning: profile.MkCommand("show running-config"),
			GetStartup: profile.MkCommand("show startup-config"),
		},
	})
	t.Run("running_config", func(t *testing.T) {
		result, err := conn.RetrieveRunningConfig(context.Background())
		if assert.NoError(t, err) {
			assert.Equal(t, expectedConfig, string(result.Output))
		}
	})
	t.Run("startup_config", func(t *testing.T) {
		result, err := conn.RetrieveStartupConfig(context.Background())
		if assert.NoError(t, err) {
			assert.Equal(t, expectedConfig, string(result.Output))
		}
	})
}

func TestSSHConnector_MissingSSHConfig(t *testing.T) {
	device := &ncmconfig.DeviceInstance{}
	_, err := NewSSHConnector(device)
	assert.ErrorContains(t, err, "missing ssh client config")
}

func TestSSHConnector_InvalidSSHConfig(t *testing.T) {
	device := &ncmconfig.DeviceInstance{
		IPAddress: "127.0.0.1",
		Auth: ncmconfig.AuthCredentials{
			Username: "nobody",
			Password: "wrong",
			Port:     "22",
			Protocol: "tcp",
			SSH: &ncmconfig.SSHConfig{
				Ciphers: []string{ssh.InsecureCipherAES128CBC},
			},
		},
	}
	_, err := NewSSHConnector(device)
	assert.ErrorContains(t, err, "unsupported cipher")
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
			name: "legacy cipher forbidden",
			config: &ncmconfig.SSHConfig{
				Ciphers:           []string{ssh.InsecureCipherAES128CBC},
				KeyExchanges:      []string{validKex},
				HostKeyAlgorithms: []string{validHostKey},
			},
			errContains: "unsupported cipher",
		},
		{
			name: "legacy cipher allowed",
			config: &ncmconfig.SSHConfig{
				Ciphers:               []string{ssh.InsecureCipherAES128CBC},
				KeyExchanges:          []string{validKex},
				HostKeyAlgorithms:     []string{validHostKey},
				AllowLegacyAlgorithms: true,
			},
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
			err := ValidateSSHConfig(tt.config)

			if tt.errContains != "" {
				if assert.Error(t, err) {
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
			expectedAuthMethods: 2,
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
			name: "success: 2 auth methods, user+pass + private key",
			auth: ncmconfig.AuthCredentials{
				Username:       "test",
				Password:       "hunter2",
				PrivateKeyFile: privateKeyPath,
			},
			expectedAuthMethods: 3,
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
