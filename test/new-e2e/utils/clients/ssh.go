// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clients

import (
	"time"

	"github.com/cenkalti/backoff"
	"golang.org/x/crypto/ssh"
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
	privateKeyAuth, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return nil, nil, err
	}

	sshConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(privateKeyAuth)},
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
