// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

type vmClient struct {
	client *ssh.Client
	t      *testing.T
}

func newVMClient(t *testing.T, sshKey string, connection *utils.Connection) (*vmClient, error) {
	client, _, err := clients.GetSSHClient(
		connection.User,
		fmt.Sprintf("%s:%d", connection.Host, 22),
		sshKey,
		2*time.Second, 5)
	return &vmClient{
		client: client,
		t:      t,
	}, err
}

// ExecuteWithError executes a command and returns an error if any.
func (vmClient *vmClient) ExecuteWithError(command string) (string, error) {
	output, err := clients.ExecuteCommand(vmClient.client, command)
	if err != nil {
		return "", fmt.Errorf("%v: %v", output, err)
	}
	return output, nil
}

// Execute execute a command and asserts there is no error.
func (vmClient *vmClient) Execute(command string) string {
	output, err := vmClient.ExecuteWithError(command)
	require.NoError(vmClient.t, err)
	return output
}
