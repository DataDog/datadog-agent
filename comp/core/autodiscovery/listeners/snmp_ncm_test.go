// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package listeners

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/snmp"
)

// newNCMTestListener returns a listener wired with a temp confd_path and the given namespace.
func newNCMTestListener(t *testing.T, namespace string) (*SNMPListener, string) {
	t.Helper()
	confdPath := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("confd_path", confdPath)

	l := &SNMPListener{
		config: snmp.ListenerConfig{Namespace: namespace, ConfdPath: confdPath},
	}
	dest := filepath.Join(confdPath, ncmConfigDirName, ncmConfigFileName)
	return l, dest
}

func readNCMFile(t *testing.T, dest string) ncmFileConfig {
	t.Helper()
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	var got ncmFileConfig
	require.NoError(t, yaml.Unmarshal(data, &got))
	return got
}

func TestNCMConfigEnabled(t *testing.T) {
	withCreds := &SNMPListener{
		config: snmp.ListenerConfig{
			Configs: []snmp.Config{
				{Network: "10.0.0.0/24"},
				{Network: "10.1.0.0/24", NCM: []snmp.NCMCredential{{User: "admin", Password: "secret"}}},
			},
		},
	}
	assert.True(t, withCreds.ncmConfigEnabled())

	withoutCreds := &SNMPListener{
		config: snmp.ListenerConfig{
			Configs: []snmp.Config{{Network: "10.0.0.0/24"}},
		},
	}
	assert.False(t, withoutCreds.ncmConfigEnabled())
}

func TestRecordNCMDeviceWritesFile(t *testing.T) {
	l, dest := newNCMTestListener(t, "myns")

	creds := []snmp.NCMCredential{
		{User: "admin", Password: "secret"},
		{User: "netops", Password: "hunter2"},
	}
	l.recordNCMDevice("a1", "10.10.0.5", creds, true)

	got := readNCMFile(t, dest)
	assert.Equal(t, "myns", got.InitConfig.Namespace)
	// No global init_config.ssh configured and no per-credential override: nothing is injected.
	assert.Nil(t, got.InitConfig.SSH)

	// One instance per (device IP x credential), no auth.ssh (strict pass-through).
	assert.ElementsMatch(t, []ncmInstance{
		{IPAddress: "10.10.0.5", Auth: ncmAuth{Username: "admin", Password: "secret"}},
		{IPAddress: "10.10.0.5", Auth: ncmAuth{Username: "netops", Password: "hunter2"}},
	}, got.Instances)
}

func TestRecordNCMDeviceMultipleDevices(t *testing.T) {
	l, dest := newNCMTestListener(t, "myns")

	l.recordNCMDevice("a1", "10.10.0.5", []snmp.NCMCredential{{User: "admin", Password: "secret"}}, true)
	l.recordNCMDevice("a2", "10.10.0.6", []snmp.NCMCredential{{User: "admin", Password: "secret"}}, true)

	got := readNCMFile(t, dest)
	assert.ElementsMatch(t, []ncmInstance{
		{IPAddress: "10.10.0.5", Auth: ncmAuth{Username: "admin", Password: "secret"}},
		{IPAddress: "10.10.0.6", Auth: ncmAuth{Username: "admin", Password: "secret"}},
	}, got.Instances)
}

func TestRecordNCMDeviceSkipsUnchanged(t *testing.T) {
	l, dest := newNCMTestListener(t, "myns")

	creds := []snmp.NCMCredential{{User: "admin", Password: "secret"}}

	// First record writes the file.
	l.recordNCMDevice("a1", "10.10.0.5", creds, true)
	_, err := os.Stat(dest)
	require.NoError(t, err)

	// Delete the file, then record the same device with identical credentials. Since the
	// device entry is unchanged, recordNCMDevice must skip the write and NOT recreate the file.
	require.NoError(t, os.Remove(dest))
	l.recordNCMDevice("a1", "10.10.0.5", creds, true)
	_, err = os.Stat(dest)
	assert.True(t, os.IsNotExist(err), "file should not be rewritten when the device entry is unchanged")
}

func TestRecordNCMDeviceNoCredentialsIsNoop(t *testing.T) {
	l, dest := newNCMTestListener(t, "myns")

	// Device on a subnet without NCM credentials must not be recorded or written.
	l.recordNCMDevice("b1", "10.20.0.5", nil, true)

	assert.Empty(t, l.ncmDevices)
	_, err := os.Stat(dest)
	assert.True(t, os.IsNotExist(err), "no file should be written when device has no credentials")
}

func TestRemoveNCMDevice(t *testing.T) {
	l, dest := newNCMTestListener(t, "myns")

	l.recordNCMDevice("a1", "10.10.0.5", []snmp.NCMCredential{{User: "admin", Password: "secret"}}, true)
	l.recordNCMDevice("a2", "10.10.0.6", []snmp.NCMCredential{{User: "admin", Password: "secret"}}, true)

	// Removing one device leaves the other in the file.
	l.removeNCMDevice("a1")
	got := readNCMFile(t, dest)
	assert.ElementsMatch(t, []ncmInstance{
		{IPAddress: "10.10.0.6", Auth: ncmAuth{Username: "admin", Password: "secret"}},
	}, got.Instances)

	// Removing the last device removes the file entirely (rather than an instance-less config).
	l.removeNCMDevice("a2")
	_, err := os.Stat(dest)
	assert.True(t, os.IsNotExist(err), "file should be removed when no devices remain")
}

func TestRemoveNCMDeviceUnknownIsNoop(t *testing.T) {
	l, dest := newNCMTestListener(t, "myns")

	// Removing a device that was never recorded should not create or touch the file.
	l.removeNCMDevice("nope")
	_, err := os.Stat(dest)
	assert.True(t, os.IsNotExist(err))
}

func TestWriteNCMConfigDefaultNamespace(t *testing.T) {
	l, dest := newNCMTestListener(t, "")

	l.recordNCMDevice("a1", "10.10.0.5", []snmp.NCMCredential{{User: "admin", Password: "secret"}}, true)

	got := readNCMFile(t, dest)
	assert.Equal(t, defaultNCMNamespace, got.InitConfig.Namespace)
	assert.Len(t, got.Instances, 1)
}

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

// TestNCMSSHGlobalDefaultFallback verifies that, with a global init_config.ssh and a credential
// without an ssh override, the global block is written to init_config.ssh and the instance has no
// auth.ssh (the device inherits the global on the NCM side).
func TestNCMSSHGlobalDefaultFallback(t *testing.T) {
	l, dest := newNCMTestListener(t, "myns")
	l.config.InitConfig = snmp.NCMInitConfig{
		SSH: &snmp.NCMSSHConfig{
			KnownHostsPath: "/etc/ssh/known_hosts",
			Ciphers:        []string{"aes256-gcm@openssh.com"},
		},
	}

	l.recordNCMDevice("a1", "10.10.0.5", []snmp.NCMCredential{{User: "admin", Password: "secret"}}, true)

	got := readNCMFile(t, dest)
	require.NotNil(t, got.InitConfig.SSH)
	assert.Equal(t, "/etc/ssh/known_hosts", got.InitConfig.SSH.KnownHostsPath)
	assert.Equal(t, []string{"aes256-gcm@openssh.com"}, got.InitConfig.SSH.Ciphers)

	require.Len(t, got.Instances, 1)
	assert.Nil(t, got.Instances[0].Auth.SSH, "credential without ssh override must not emit auth.ssh")
}

// TestNCMSSHCredentialOverrideMerge verifies field-by-field precedence: a credential ssh block
// overrides only the fields it sets and inherits the rest from the global init_config.ssh.
func TestNCMSSHCredentialOverrideMerge(t *testing.T) {
	l, dest := newNCMTestListener(t, "myns")
	l.config.InitConfig = snmp.NCMInitConfig{
		SSH: &snmp.NCMSSHConfig{
			KnownHostsPath: "/etc/ssh/known_hosts",
			Ciphers:        []string{"aes256-gcm@openssh.com"},
			Timeout:        intPtr(30),
		},
	}

	creds := []snmp.NCMCredential{
		{
			User:     "admin",
			Password: "secret",
			SSH: &snmp.NCMSSHConfig{
				Ciphers: []string{"aes128-ctr"},
			},
		},
	}
	l.recordNCMDevice("a1", "10.10.0.5", creds, true)

	got := readNCMFile(t, dest)
	require.Len(t, got.Instances, 1)
	authSSH := got.Instances[0].Auth.SSH
	require.NotNil(t, authSSH)
	// Overridden field comes from the credential.
	assert.Equal(t, []string{"aes128-ctr"}, authSSH.Ciphers)
	// Non-overridden fields are inherited from the global init_config.ssh.
	assert.Equal(t, "/etc/ssh/known_hosts", authSSH.KnownHostsPath)
	require.NotNil(t, authSSH.Timeout)
	assert.Equal(t, 30, *authSSH.Timeout)
	// Unset everywhere -> absent.
	assert.Nil(t, authSSH.InsecureSkipVerify)
}

// TestNCMSSHStrictPassthrough verifies that only the fields actually specified are written: a
// credential ssh block with no global default emits exactly those fields and omits the rest.
func TestNCMSSHStrictPassthrough(t *testing.T) {
	l, dest := newNCMTestListener(t, "myns")

	creds := []snmp.NCMCredential{
		{
			User:     "admin",
			Password: "secret",
			SSH: &snmp.NCMSSHConfig{
				KnownHostsPath: "/etc/ssh/known_hosts",
				Ciphers:        []string{"aes128-ctr"},
			},
		},
	}
	l.recordNCMDevice("a1", "10.10.0.5", creds, true)

	// No global ssh -> init_config.ssh omitted entirely.
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "insecure_skip_verify")
	assert.NotContains(t, string(data), "timeout")
	assert.NotContains(t, string(data), "key_exchanges")

	got := readNCMFile(t, dest)
	assert.Nil(t, got.InitConfig.SSH)
	require.Len(t, got.Instances, 1)
	authSSH := got.Instances[0].Auth.SSH
	require.NotNil(t, authSSH)
	assert.Equal(t, "/etc/ssh/known_hosts", authSSH.KnownHostsPath)
	assert.Equal(t, []string{"aes128-ctr"}, authSSH.Ciphers)
	assert.Nil(t, authSSH.InsecureSkipVerify)
	assert.Nil(t, authSSH.Timeout)
	assert.Empty(t, authSSH.KeyExchanges)
	assert.Empty(t, authSSH.HostKeyAlgorithms)
	assert.Nil(t, authSSH.AllowLegacyAlgorithms)
}

// TestNCMSSHChangeRewrites verifies that a change in a credential's ssh block triggers a rewrite,
// while re-recording an identical block (same ssh) is skipped.
func TestNCMSSHChangeRewrites(t *testing.T) {
	l, dest := newNCMTestListener(t, "myns")

	creds := []snmp.NCMCredential{
		{User: "admin", Password: "secret", SSH: &snmp.NCMSSHConfig{InsecureSkipVerify: boolPtr(true)}},
	}
	l.recordNCMDevice("a1", "10.10.0.5", creds, true)
	require.FileExists(t, dest)

	// Re-recording with an identical ssh block must be a no-op (file not recreated after removal).
	require.NoError(t, os.Remove(dest))
	sameCreds := []snmp.NCMCredential{
		{User: "admin", Password: "secret", SSH: &snmp.NCMSSHConfig{InsecureSkipVerify: boolPtr(true)}},
	}
	l.recordNCMDevice("a1", "10.10.0.5", sameCreds, true)
	_, err := os.Stat(dest)
	assert.True(t, os.IsNotExist(err), "identical ssh block must not trigger a rewrite")

	// Changing an ssh field must trigger a rewrite.
	changedCreds := []snmp.NCMCredential{
		{User: "admin", Password: "secret", SSH: &snmp.NCMSSHConfig{InsecureSkipVerify: boolPtr(false)}},
	}
	l.recordNCMDevice("a1", "10.10.0.5", changedCreds, true)
	got := readNCMFile(t, dest)
	require.Len(t, got.Instances, 1)
	require.NotNil(t, got.Instances[0].Auth.SSH)
	require.NotNil(t, got.Instances[0].Auth.SSH.InsecureSkipVerify)
	assert.False(t, *got.Instances[0].Auth.SSH.InsecureSkipVerify)
}
