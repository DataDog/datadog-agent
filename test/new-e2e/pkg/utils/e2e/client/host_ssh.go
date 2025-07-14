// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func execute(sshClient *ssh.Client, command string) (string, error) {
	session, err := sshClient.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	stdout, err := session.CombinedOutput(command)
	return string(stdout), err
}

func start(sshClient *ssh.Client, command string) (*ssh.Session, io.WriteCloser, io.Reader, error) {
	session, err := sshClient.NewSession()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create session: %v", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	err = session.Start(command)
	return session, stdin, stdout, err
}

func getSSHClient(user, host string, privateKey, privateKeyPassphrase []byte) (*ssh.Client, error) {
	var auth ssh.AuthMethod

	if len(privateKey) > 0 {
		var privateKeyAuth ssh.Signer
		var err error

		if len(privateKeyPassphrase) > 0 {
			privateKeyAuth, err = ssh.ParsePrivateKeyWithPassphrase(privateKey, privateKeyPassphrase)
		} else {
			privateKeyAuth, err = ssh.ParsePrivateKey(privateKey)
		}

		if err != nil {
			return nil, err
		}
		auth = ssh.PublicKeys(privateKeyAuth)
	} else {
		// Use the ssh agent
		conn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
		if err != nil {
			return nil, fmt.Errorf("no ssh key provided and cannot connect to the ssh agent: %v", err)
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
		return nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, err
	}
	err = session.Close()
	if err != nil {
		return nil, err
	}

	return client, nil
}

func copyFileFromIoReader(sftpClient *sftp.Client, srcFile io.Reader, dst string) error {
	lastSlashIdx := strings.LastIndex(dst, "/")
	if lastSlashIdx >= 0 {
		// Ensure the target directory exists
		// otherwise sftpClient.Create will return an error
		err := sftpClient.MkdirAll(dst[:lastSlashIdx])
		if err != nil {
			return err
		}
	}

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

func copyFile(sftpClient *sftp.Client, src string, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	return copyFileFromIoReader(sftpClient, srcFile, dst)
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
