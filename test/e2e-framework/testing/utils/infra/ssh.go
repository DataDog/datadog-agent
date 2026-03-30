// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infra implements utilities to interact with a Pulumi infrastructure
package infra

import (
	"fmt"
	"io"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const (
	NR_SSH_COMMAND_RETRIES = 3
)

// SshConnectToInstance connects using ssh protocol to the given ip:port using
// the given user.
func SshConnectToInstance(ip, port, user string) (*ssh.Client, error) {
	auth := []ssh.AuthMethod{}

	if sshAgentSocket, found := os.LookupEnv("SSH_AUTH_SOCK"); found {
		sshAgent, err := net.Dial("unix", sshAgentSocket)
		if err != nil {
			return nil, fmt.Errorf("failed to dial SSH agent: %w", err)
		}
		defer sshAgent.Close()

		auth = append(auth, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
	}

	if sshKeyPath, found := os.LookupEnv("E2E_AWS_PRIVATE_KEY_PATH"); found {
		sshKey, err := os.ReadFile(sshKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read SSH key: %w", err)
		}

		signer, err := ssh.ParsePrivateKey(sshKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SSH key: %w", err)
		}

		auth = append(auth, ssh.PublicKeys(signer))
	}

	if sshKeyPath, found := os.LookupEnv("E2E_GCP_PRIVATE_KEY_PATH"); found {
		sshKey, err := os.ReadFile(sshKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read SSH key: %w", err)
		}

		signer, err := ssh.ParsePrivateKey(sshKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SSH key: %w", err)
		}

		auth = append(auth, ssh.PublicKeys(signer))
	}

	return ssh.Dial("tcp", ip+":"+port, &ssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
}

// SshRunCommand runs the given command using the given ssh client.
// retries on error. Returns the last error if retries exceeded.
func SshRunCommand(sshClient *ssh.Client, command string, logger io.Writer) ([]byte, error) {
	fmt.Fprintf(logger, "Command: '%s'\n", command)

	var err error
	for range NR_SSH_COMMAND_RETRIES {
		sshSession, err := sshClient.NewSession()
		if err != nil {
			return nil, err
		}

		output, err := sshSession.CombinedOutput(command)
		if err == nil {
			return output, nil
		}
	}

	return nil, err
}
