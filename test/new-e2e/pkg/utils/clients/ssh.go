// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clients

import (
	"fmt"
	"net"
	"os"
	"path"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// GetSSHClient returns an ssh Client for the specified host
func GetSSHClient(user, host string, privateKey []byte, retryInterval time.Duration, maxRetries uint64) (client *ssh.Client, session *ssh.Session, err error) {
	err = backoff.Retry(func() error {
		client, session, err = getSSHClient(user, host, privateKey)
		return err
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(retryInterval), maxRetries))

	return
}

func getSSHClient(user, host string, privateKey []byte) (*ssh.Client, *ssh.Session, error) {
	var auth ssh.AuthMethod

	if privateKey != nil {
		privateKeyAuth, err := ssh.ParsePrivateKey(privateKey)
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

// ExecuteCommand creates a session on an ssh client and runs a command.
// It returns the command output and errors
func ExecuteCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}

	stdout, err := session.CombinedOutput(command)

	return string(stdout), err
}

// CopyFile create a sftp session and copy a single file to the remote host through SSH
func CopyFile(client *ssh.Client, src string, dst string) error {

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	return copyFile(sftpClient, src, dst)
}

// CopyFolder create a sftp session and copy a folder to remote host through SSH
func CopyFolder(client *ssh.Client, srcFolder string, dstFolder string) error {

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	return copyFolder(sftpClient, srcFolder, dstFolder)
}

func copyFolder(sftpClient *sftp.Client, srcFolder string, dstFolder string) error {

	folderContent, err := os.ReadDir(srcFolder)
	if err != nil {
		return err
	}

	if err := sftpClient.MkdirAll(dstFolder); err != nil {
		return err
	}

	for _, d := range folderContent {
		if !d.IsDir() {
			err := copyFile(sftpClient, path.Join(srcFolder, d.Name()), path.Join(dstFolder, d.Name()))
			if err != nil {
				return err
			}
		} else {
			err = copyFolder(sftpClient, path.Join(srcFolder, d.Name()), path.Join(dstFolder, d.Name()))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(sftpClient *sftp.Client, src string, dst string) error {

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := sftpClient.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := dstFile.ReadFrom(srcFile); err != nil {
		return err
	}
	return nil

}
