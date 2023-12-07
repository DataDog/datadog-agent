package components

import (
	"fmt"
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
	h.context.T().Logf("connecting to remote VM at %s@%s", h.Username, h.Host)

	var privateSSHKey []byte
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

	h.client, err = clients.GetSSHClient(
		h.Username,
		fmt.Sprintf("%s:%d", h.Host, 22),
		privateSSHKey,
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
