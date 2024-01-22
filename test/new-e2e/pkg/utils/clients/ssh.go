// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clients

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
func GetSSHClient(user, host string, privateKey, privateKeyPassphrase []byte, retryInterval time.Duration, maxRetries uint64) (client *ssh.Client, err error) {
	err = backoff.Retry(func() error {
		client, err = getSSHClient(user, host, privateKey, privateKeyPassphrase)
		return err
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(retryInterval), maxRetries))

	return
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

// ExecuteCommand creates a session on an ssh client and runs a command.
// It returns the command output and errors
func ExecuteCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
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

// GetFile create a sftp session and copy a single file from the remote host through SSH
func GetFile(client *ssh.Client, src string, dst string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	// remote
	fsrc, err := sftpClient.Open(src)
	if err != nil {
		return err
	}
	defer fsrc.Close()

	// local
	fdst, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE, 0o640)
	if err != nil {
		return err
	}
	defer fdst.Close()

	_, err = fsrc.WriteTo(fdst)
	return err
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

// FileExists create a sftp session to and returns true if the file exists and is a regular file
func FileExists(client *ssh.Client, path string) (bool, error) {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return false, err
	}
	defer sftpClient.Close()

	info, err := sftpClient.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	return info.Mode().IsRegular(), nil
}

// ReadFile reads the content of the file, return bytes read and error if any
func ReadFile(client *ssh.Client, path string) ([]byte, error) {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, err
	}
	defer sftpClient.Close()

	f, err := sftpClient.Open(path)
	if err != nil {
		return nil, err
	}

	var content bytes.Buffer
	_, err = io.Copy(&content, f)
	if err != nil {
		return content.Bytes(), err
	}

	return content.Bytes(), nil
}

// WriteFile write content to the file and returns the number of bytes written and error if any
func WriteFile(client *ssh.Client, path string, content []byte) (int64, error) {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return 0, err
	}
	defer sftpClient.Close()

	f, err := sftpClient.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	reader := bytes.NewReader(content)
	return io.Copy(f, reader)
}

// AppendFile append content to the file and returns the number of bytes appened and error if any
func AppendFile(client *ssh.Client, os, path string, content []byte) (int64, error) {
	if os == "linux" {
		return appendWithSudo(client, path, content)
	}
	return appendWithSftp(client, path, content)

}

// appendWithSudo appends content to the file using sudo tee for Linux environment
func appendWithSudo(client *ssh.Client, path string, content []byte) (int64, error) {
	cmd := fmt.Sprintf("echo '%s' | sudo tee -a %s", string(content), path)
	session, err := client.NewSession()
	if err != nil {
		return 0, err
	}
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(cmd); err != nil {
		return 0, err
	}

	return int64(len(content)), nil
}

// appendWithSftp appends content to the file using sftp for Windows environment
func appendWithSftp(client *ssh.Client, path string, content []byte) (int64, error) {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return 0, err
	}
	defer sftpClient.Close()

	// Open the file in append mode and create it if it doesn't exist
	f, err := sftpClient.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	reader := bytes.NewReader(content)
	written, err := io.Copy(f, reader)
	if err != nil {
		return 0, err
	}

	return written, nil
}

// ReadDir returns list of directory entries in path
func ReadDir(client *ssh.Client, path string) ([]fs.DirEntry, error) {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, err
	}
	defer sftpClient.Close()

	infos, err := sftpClient.ReadDir(path)
	if err != nil {
		return nil, err
	}

	entries := make([]fs.DirEntry, 0, len(infos))
	for _, info := range infos {
		entry := fs.FileInfoToDirEntry(info)
		entries = append(entries, entry)
	}

	return entries, nil
}

// Lstat returns a FileInfo structure describing path.
// if path is a symbolic link, the FileInfo structure describes the symbolic link.
func Lstat(client *ssh.Client, path string) (fs.FileInfo, error) {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, err
	}
	defer sftpClient.Close()

	return sftpClient.Lstat(path)
}

// MkdirAll creates the specified directory along with any necessary parents.
// If the path is already a directory, does nothing and returns nil.
// Otherwise returns an error if any.
func MkdirAll(client *ssh.Client, path string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	return sftpClient.MkdirAll(path)
}

// Remove removes the specified file or directory.
// Returns an error if file or directory does not exist, or if the directory is not empty.
func Remove(client *ssh.Client, path string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	return sftpClient.Remove(path)
}

// RemoveAll recursively removes all files/folders in the specified directory.
// Returns an error if the directory does not exist.
func RemoveAll(client *ssh.Client, path string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	return sftpClient.RemoveAll(path)
}
