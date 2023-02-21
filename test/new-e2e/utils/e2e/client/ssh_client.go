// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"golang.org/x/crypto/ssh"
)

type sshClient struct {
	client *ssh.Client
}

func newSSHClient(auth *Authentification, connection *utils.Connection) (*sshClient, error) {
	client, _, err := clients.GetSSHClient(
		connection.User,
		fmt.Sprintf("%s:%d", connection.Host, 22),
		auth.SSHKey,
		2*time.Second, 5)
	return &sshClient{
		client: client,
	}, err
}

// Execute a command
func (vm *sshClient) Execute(command string) (string, error) {
	return clients.ExecuteCommand(vm.client, command)
}
