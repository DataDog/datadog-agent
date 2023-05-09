// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clients

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// GetSSHClient returns an ssh Client for the specified host
func GetSSHClient(user, host, privateKey string, retryInterval time.Duration, maxRetries uint64) (client *ssh.Client, session *ssh.Session, err error) {
	err = backoff.Retry(func() error {
		client, session, err = getSSHClient(user, host, privateKey)
		return err
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(retryInterval), maxRetries))

	return
}

func getSSHClient(user, host, privateKey string) (*ssh.Client, *ssh.Session, error) {
	var auth ssh.AuthMethod

	if privateKey != "" {
		privateKeyAuth, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			return nil, nil, err
		}
		auth = ssh.PublicKeys(privateKeyAuth)
	} else {
		// Use the ssh agent
		conn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
		if err != nil {
			return nil, nil, fmt.Errorf("no ssh key provided and cannot connect to the ssh agent: %v", err)
		}
		defer conn.Close()
		sshAgent := agent.NewClient(conn)
		auth = ssh.PublicKeysCallback(sshAgent.Signers)
	}

	sshConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", host, sshConfig)
	if err != nil {
		return nil, nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, err
	}

	return client, session, nil
}

func ExecuteCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}

	stdout, err := session.CombinedOutput(command)

	return string(stdout), err
}
