// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/clients"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/executeparams"
	"github.com/DataDog/test-infra-definitions/common/utils"
	commonos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

type vmClient struct {
	client *ssh.Client
	os     commonos.OS
	t      *testing.T
}

func newVMClient(t *testing.T, sshKey []byte, connection *utils.Connection, os commonos.OS) (*vmClient, error) {
	t.Logf("connecting to remote VM at %s:%s", connection.User, connection.Host)
	client, _, err := clients.GetSSHClient(
		connection.User,
		fmt.Sprintf("%s:%d", connection.Host, 22),
		sshKey,
		2*time.Second, 5)
	return &vmClient{
		client: client,
		os:     os,
		t:      t,
	}, err
}

// ExecuteWithError executes a command and returns an error if any.
func (vmClient *vmClient) ExecuteWithError(command string, options ...executeparams.Option) (string, error) {
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
func (vmClient *vmClient) Execute(command string, options ...executeparams.Option) string {
	output, err := vmClient.ExecuteWithError(command, options...)
	require.NoError(vmClient.t, err)
	return output
}

// CopyFile copy file to the remote host
func (vmClient *vmClient) CopyFile(src string, dst string) {
	err := clients.CopyFile(vmClient.client, src, dst)
	require.NoError(vmClient.t, err)
}

// CopyFolder copy a folder to the remote host
func (vmClient *vmClient) CopyFolder(srcFolder string, dstFolder string) {
	err := clients.CopyFolder(vmClient.client, srcFolder, dstFolder)
	require.NoError(vmClient.t, err)
}

func (vmClient *vmClient) setEnvVariables(command string, envVar executeparams.EnvVar) string {

	cmd := ""
	if vmClient.os.GetType() == commonos.WindowsType {
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
