// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/clients"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/executeparams"
	"github.com/DataDog/test-infra-definitions/common/utils"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

var _ VM = (*VMClient)(nil)

// VMClient is a type that implements [VM] interface to interact with a remote VM.
type VMClient struct {
	client     *ssh.Client
	connection *utils.Connection
	osType     componentos.Type
	t          *testing.T
}

// NewVMClient creates a new instance of VMClient.
func NewVMClient(t *testing.T, connection *utils.Connection, osType componentos.Type) (*VMClient, error) {
	t.Logf("connecting to remote VM at %s:%s", connection.User, connection.Host)
	vmClient := &VMClient{
		connection: connection,
		osType:     osType,
		t:          t,
	}

	err := vmClient.connect()

	if err != nil {
		return nil, err
	}

	return vmClient, nil
}

// ReconnectSSH recreate the SSH connection to the VM. Should be used only after VM reboot to restore the SSH connection.
func (vmClient *VMClient) ReconnectSSH() error {
	if vmClient.client != nil {
		vmClient.client.Close()
	}

	err := vmClient.connect()
	if err != nil {
		return err
	}

	return nil
}

// ExecuteWithError executes a command and returns an error if any.
func (vmClient *VMClient) ExecuteWithError(command string, options ...executeparams.Option) (string, error) {
	params, err := executeparams.NewParams(options...)
	if err != nil {
		return "", err
	}
	cmd := vmClient.setEnvVariables(command, params.EnvVariables)

	output, err := clients.ExecuteCommand(vmClient.client, cmd)
	if err != nil {
		return "", fmt.Errorf("%v: %v", output, err)
	}
	return output, nil
}

// Execute executes a command and returns its output.
func (vmClient *VMClient) Execute(command string, options ...executeparams.Option) string {
	output, err := vmClient.ExecuteWithError(command, options...)
	require.NoError(vmClient.t, err)
	return output
}

// CopyFile copy file to the remote host
func (vmClient *VMClient) CopyFile(src string, dst string) {
	err := clients.CopyFile(vmClient.client, src, dst)
	require.NoError(vmClient.t, err)
}

// CopyFolder copy a folder to the remote host
func (vmClient *VMClient) CopyFolder(srcFolder string, dstFolder string) {
	err := clients.CopyFolder(vmClient.client, srcFolder, dstFolder)
	require.NoError(vmClient.t, err)
}

// GetFile copy file from the remote host
func (vmClient *VMClient) GetFile(src string, dst string) error {
	return clients.GetFile(vmClient.client, src, dst)
}

// FileExists returns true if the file exists and is a regular file and returns an error if any
func (vmClient *VMClient) FileExists(path string) (bool, error) {
	return clients.FileExists(vmClient.client, path)
}

// ReadFile reads the content of the file, return bytes read and error if any
func (vmClient *VMClient) ReadFile(path string) ([]byte, error) {
	return clients.ReadFile(vmClient.client, path)
}

// WriteFile write content to the file and returns the number of bytes written and error if any
func (vmClient *VMClient) WriteFile(path string, content []byte) (int64, error) {
	return clients.WriteFile(vmClient.client, path, content)
}

// ReadDir returns list of directory entries in path
func (vmClient *VMClient) ReadDir(path string) ([]fs.DirEntry, error) {
	return clients.ReadDir(vmClient.client, path)
}

// Lstat returns a FileInfo structure describing path.
// if path is a symbolic link, the FileInfo structure describes the symbolic link.
func (vmClient *VMClient) Lstat(path string) (fs.FileInfo, error) {
	return clients.Lstat(vmClient.client, path)
}

// MkdirAll creates the specified directory along with any necessary parents.
// If the path is already a directory, does nothing and returns nil.
// Otherwise returns an error if any.
func (vmClient *VMClient) MkdirAll(path string) error {
	return clients.MkdirAll(vmClient.client, path)
}

// Remove removes the specified file or directory.
// Returns an error if file or directory does not exist, or if the directory is not empty.
func (vmClient *VMClient) Remove(path string) error {
	return clients.Remove(vmClient.client, path)
}

// RemoveAll recursively removes all files/folders in the specified directory.
// Returns an error if the directory does not exist.
func (vmClient *VMClient) RemoveAll(path string) error {
	return clients.RemoveAll(vmClient.client, path)
}

func (vmClient *VMClient) setEnvVariables(command string, envVar executeparams.EnvVar) string {

	cmd := ""
	if vmClient.osType == componentos.WindowsType {
		envVarSave := map[string]string{}
		for envName, envValue := range envVar {
			previousEnvVar, err := vmClient.ExecuteWithError(fmt.Sprintf("$env:%s", envName))
			if err != nil || previousEnvVar == "" {
				previousEnvVar = "null"
			}
			envVarSave[envName] = previousEnvVar

			cmd += fmt.Sprintf("$env:%s='%s'; ", envName, envValue)
		}
		cmd += fmt.Sprintf("%s; ", command)

		// Restore env variables
		for envName := range envVar {
			cmd += fmt.Sprintf("$env:%s='%s'; ", envName, envVarSave[envName])
		}
	} else {
		for envName, envValue := range envVar {
			cmd += fmt.Sprintf("%s='%s' ", envName, envValue)
		}
		cmd += command
	}
	return cmd

}

func (vmClient *VMClient) connect() error {
	var privateSSHKey, privateKeyPassphraseBytes []byte

	privateKeyPath, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.PrivateKeyPath, "")
	if err != nil {
		return err
	}

	if privateKeyPath != "" {
		privateSSHKey, err = os.ReadFile(privateKeyPath)
		if err != nil {
			return err
		}
	}

	privateKeyPassphrase, err := runner.GetProfile().SecretStore().GetWithDefault(parameters.PrivateKeyPassword, "")
	if err != nil {
		return err
	}
	if privateKeyPassphrase != "" {
		privateKeyPassphraseBytes = []byte(privateKeyPassphrase)
	}

	client, _, err := clients.GetSSHClient(
		vmClient.connection.User,
		fmt.Sprintf("%s:%d", vmClient.connection.Host, 22),
		privateSSHKey,
		privateKeyPassphraseBytes,
		2*time.Second, 5)
	if err != nil {
		return err
	}

	vmClient.client = client

	return nil
}

// GetOSType returns the operating system type of the VMClient instance.
func (vmClient *VMClient) GetOSType() componentos.Type {
	return vmClient.osType
}
