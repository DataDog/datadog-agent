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

type sshClient struct {
	client *ssh.Client
	t      *testing.T
}

func newSSHClient(t *testing.T, sshKey string, connection *utils.Connection) (*sshClient, error) {
	client, _, err := clients.GetSSHClient(
		connection.User,
		fmt.Sprintf("%s:%d", connection.Host, 22),
		sshKey,
		2*time.Second, 5)
	return &sshClient{
		client: client,
		t:      t,
	}, err
}

// ExecuteWithError executes a command and returns an error if any.
func (sshClient *sshClient) ExecuteWithError(command string) (string, error) {
	return clients.ExecuteCommand(sshClient.client, command)
}

// Execute execute a command and asserts there is no error.
func (sshClient *sshClient) Execute(command string) string {
	output, err := sshClient.ExecuteWithError(command)
	require.NoError(sshClient.t, err)
	return output
}
