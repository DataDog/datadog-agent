package components

import (
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/clients"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	osComp "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/components/remote"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const (
	// Waiting for only 10s as we expect remote to be ready when provisioning
	sshRetryInterval = 2 * time.Second
	sshMaxRetries    = 5
)

type Host struct {
	remote.HostOutput

	client  *ssh.Client
	context e2e.Context
}

var _ e2e.Initializable = &Host{}

func (h *Host) Init(ctx e2e.Context) error {
	h.context = ctx
	h.context.T().Logf("connecting to remote VM at %s@%s", h.Username, h.Address)

	var privateSSHKey []byte
	privateKeyPath, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.PrivateKeyPath, "")
	if err != nil {
		return err
	}

	privateKeyPassword, err := runner.GetProfile().SecretStore().GetWithDefault(parameters.PrivateKeyPassword, "")
	if err != nil {
		return err
	}

	if privateKeyPath != "" {
		privateSSHKey, err = os.ReadFile(privateKeyPath)
		if err != nil {
			return err
		}
	}

	h.client, err = clients.GetSSHClient(
		h.Username,
		h.Address,
		privateSSHKey,
		[]byte(privateKeyPassword),
		sshRetryInterval,
		sshMaxRetries,
	)
	if err != nil {
		return err
	}

	return nil
}

// Execute executes a command and returns an error if any.
func (h *Host) Execute(command string, options ...ExecuteOption) (string, error) {
	params, err := optional.MakeParams(options...)
	if err != nil {
		return "", err
	}
	cmd := h.buildEnvVariables(command, params.EnvVariables)

	output, err := clients.ExecuteCommand(h.client, cmd)
	if err != nil {
		return "", fmt.Errorf("%v: %v", output, err)
	}
	return output, nil
}

// MustExecute executes a command and returns its output.
func (h *Host) MustExecute(command string, options ...ExecuteOption) string {
	output, err := h.Execute(command, options...)
	require.NoError(h.context.T(), err)
	return output
}

// CopyFile copy file to the remote host
func (h *Host) CopyFile(src string, dst string) {
	err := clients.CopyFile(h.client, src, dst)
	require.NoError(h.context.T(), err)
}

// CopyFolder copy a folder to the remote host
func (h *Host) CopyFolder(srcFolder string, dstFolder string) {
	err := clients.CopyFolder(h.client, srcFolder, dstFolder)
	require.NoError(h.context.T(), err)
}

// FileExists returns true if the file exists and is a regular file and returns an error if any
func (h *Host) FileExists(path string) (bool, error) {
	return clients.FileExists(h.client, path)
}

// ReadFile reads the content of the file, return bytes read and error if any
func (h *Host) ReadFile(path string) ([]byte, error) {
	return clients.ReadFile(h.client, path)
}

// WriteFile write content to the file and returns the number of bytes written and error if any
func (h *Host) WriteFile(path string, content []byte) (int64, error) {
	return clients.WriteFile(h.client, path, content)
}

// ReadDir returns list of directory entries in path
func (h *Host) ReadDir(path string) ([]fs.DirEntry, error) {
	return clients.ReadDir(h.client, path)
}

// Lstat returns a FileInfo structure describing path.
// if path is a symbolic link, the FileInfo structure describes the symbolic link.
func (h *Host) Lstat(path string) (fs.FileInfo, error) {
	return clients.Lstat(h.client, path)
}

// MkdirAll creates the specified directory along with any necessary parents.
// If the path is already a directory, does nothing and returns nil.
// Otherwise returns an error if any.
func (h *Host) MkdirAll(path string) error {
	return clients.MkdirAll(h.client, path)
}

// Remove removes the specified file or directory.
// Returns an error if file or directory does not exist, or if the directory is not empty.
func (h *Host) Remove(path string) error {
	return clients.Remove(h.client, path)
}

// RemoveAll recursively removes all files/folders in the specified directory.
// Returns an error if the directory does not exist.
func (h *Host) RemoveAll(path string) error {
	return clients.RemoveAll(h.client, path)
}

func (h *Host) buildEnvVariables(command string, envVar EnvVar) string {
	cmd := ""
	if h.OSFamily == osComp.WindowsFamily {
		envVarSave := map[string]string{}
		for envName, envValue := range envVar {
			previousEnvVar, err := h.Execute(fmt.Sprintf("$env:%s", envName))
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
