// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/test-infra-definitions/components/remote"

	oscomp "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/cenkalti/backoff"
	"github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const (
	// Waiting for only 10s as we expect remote to be ready when provisioning
	sshRetryInterval = 2 * time.Second
	sshMaxRetries    = 20
)

type buildCommandFn func(host *Host, command string, envVars EnvVar) string

type convertPathSeparatorFn func(string) string

// A Host client that is connected to an [ssh.Client].
type Host struct {
	client *ssh.Client

	context              e2e.Context
	username             string
	host                 string
	privateKey           []byte
	privateKeyPassphrase []byte
	buildCommand         buildCommandFn
	convertPathSeparator convertPathSeparatorFn
	osFamily             oscomp.Family
}

// NewHost creates a new ssh client to connect to a remote host with
// reconnect retry logic
func NewHost(context e2e.Context, hostOutput remote.HostOutput) (*Host, error) {
	var privateSSHKey []byte
	privateKeyPath, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.PrivateKeyPath, "")
	if err != nil {
		return nil, err
	}

	privateKeyPassword, err := runner.GetProfile().SecretStore().GetWithDefault(parameters.PrivateKeyPassword, "")
	if err != nil {
		return nil, err
	}

	if privateKeyPath != "" {
		privateSSHKey, err = os.ReadFile(privateKeyPath)
		if err != nil {
			return nil, err
		}
	}

	host := &Host{
		context:              context,
		username:             hostOutput.Username,
		host:                 fmt.Sprintf("%s:%d", hostOutput.Address, 22),
		privateKey:           privateSSHKey,
		privateKeyPassphrase: []byte(privateKeyPassword),
		buildCommand:         buildCommandFactory(hostOutput.OSFamily),
		convertPathSeparator: convertPathSeparatorFactory(hostOutput.OSFamily),
		osFamily:             hostOutput.OSFamily,
	}
	err = host.Reconnect()
	return host, err
}

// Reconnect closes the current ssh client and creates a new one, with retries.
func (h *Host) Reconnect() error {
	if h.client != nil {
		_ = h.client.Close()
	}
	return backoff.Retry(func() error {
		client, err := getSSHClient(h.username, h.host, h.privateKey, h.privateKeyPassphrase)
		if err != nil {
			return err
		}
		h.client = client
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(sshRetryInterval), sshMaxRetries))
}

// Execute executes a command and returns an error if any.
func (h *Host) Execute(command string, options ...ExecuteOption) (string, error) {
	params, err := optional.MakeParams(options...)
	if err != nil {
		return "", err
	}
	command = h.buildCommand(h, command, params.EnvVariables)
	return h.executeAndReconnectOnError(command)
}

func (h *Host) executeAndReconnectOnError(command string) (string, error) {
	stdout, err := execute(h.client, command)
	if err != nil && strings.Contains(err.Error(), "failed to create session:") {
		err = h.Reconnect()
		if err != nil {
			return "", err
		}
		stdout, err = execute(h.client, command)
	}
	if err != nil {
		return "", fmt.Errorf("%v: %v", stdout, err)
	}
	return stdout, err
}

// MustExecute executes a command and requires no error.
func (h *Host) MustExecute(command string, options ...ExecuteOption) string {
	stdout, err := h.Execute(command, options...)
	require.NoError(h.context.T(), err)
	return stdout
}

// CopyFile create a sftp session and copy a single file to the remote host through SSH
func (h *Host) CopyFile(src string, dst string) {
	dst = h.convertPathSeparator(dst)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()
	err := copyFile(sftpClient, src, dst)
	require.NoError(h.context.T(), err)
}

// CopyFolder create a sftp session and copy a folder to remote host through SSH
func (h *Host) CopyFolder(srcFolder string, dstFolder string) error {
	dstFolder = h.convertPathSeparator(dstFolder)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()
	return copyFolder(sftpClient, srcFolder, dstFolder)
}

// FileExists create a sftp session to and returns true if the file exists and is a regular file
func (h *Host) FileExists(path string) (bool, error) {
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
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

// GetFile create a sftp session and copy a single file from the remote host through SSH
func (h *Host) GetFile(src string, dst string) error {
	dst = h.convertPathSeparator(dst)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()

	// remote
	fsrc, err := sftpClient.Open(src)
	if err != nil {
		return err
	}
	defer fsrc.Close()

	// local
	fdst, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer fdst.Close()

	_, err = fsrc.WriteTo(fdst)
	return err
}

// ReadFile reads the content of the file, return bytes read and error if any
func (h *Host) ReadFile(path string) ([]byte, error) {
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
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
func (h *Host) WriteFile(path string, content []byte) (int64, error) {
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
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
func (h *Host) AppendFile(os, path string, content []byte) (int64, error) {
	path = h.convertPathSeparator(path)
	if os == "linux" {
		return h.appendWithSudo(path, content)
	}
	return h.appendWithSftp(path, content)
}

// ReadDir returns list of directory entries in path
func (h *Host) ReadDir(path string) ([]fs.DirEntry, error) {
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()

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
func (h *Host) Lstat(path string) (fs.FileInfo, error) {
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()

	return sftpClient.Lstat(path)
}

// MkdirAll creates the specified directory along with any necessary parents.
// If the path is already a directory, does nothing and returns nil.
// Otherwise returns an error if any.
func (h *Host) MkdirAll(path string) error {
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()

	return sftpClient.MkdirAll(path)
}

// Remove removes the specified file or directory.
// Returns an error if file or directory does not exist, or if the directory is not empty.
func (h *Host) Remove(path string) error {
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()

	return sftpClient.Remove(path)
}

// RemoveAll recursively removes all files/folders in the specified directory.
// Returns an error if the directory does not exist.
func (h *Host) RemoveAll(path string) error {
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()

	return sftpClient.RemoveAll(path)
}

// DialPort creates a connection from the remote host to its `port`.
func (h *Host) DialPort(port uint16) (net.Conn, error) {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	protocol := "tcp"
	// TODO add context to host
	context := context.Background()
	connection, err := h.client.DialContext(context, protocol, address)
	if err != nil {
		err = h.Reconnect()
		if err != nil {
			return nil, err
		}
		connection, err = h.client.DialContext(context, protocol, address)
	}
	return connection, err
}

// GetTmpFolder returns the temporary folder path for the host
func (h *Host) GetTmpFolder() (string, error) {
	switch osFamily := h.osFamily; osFamily {
	case oscomp.WindowsFamily:
		return h.Execute("echo %TEMP%")
	case oscomp.LinuxFamily:
		return "/tmp", nil
	default:
		return "", errors.ErrUnsupported
	}
}

// GetLogsFolder returns the logs folder path for the host
func (h *Host) GetLogsFolder() (string, error) {
	switch osFamily := h.osFamily; osFamily {
	case oscomp.WindowsFamily:
		return `C:\ProgramData\Datadog\logs`, nil
	case oscomp.LinuxFamily:
		return "/var/log/datadog/", nil
	case oscomp.MacOSFamily:
		return "/opt/datadog-agent/logs", nil
	default:
		return "", errors.ErrUnsupported
	}
}

// appendWithSudo appends content to the file using sudo tee for Linux environment
func (h *Host) appendWithSudo(path string, content []byte) (int64, error) {
	cmd := fmt.Sprintf("echo '%s' | sudo tee -a %s", string(content), path)
	output, err := h.Execute(cmd)
	if err != nil {
		return 0, err
	}
	return int64(len(output)), nil
}

// appendWithSftp appends content to the file using sftp for Windows environment
func (h *Host) appendWithSftp(path string, content []byte) (int64, error) {
	sftpClient := h.getSFTPClient()
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

func (h *Host) getSFTPClient() *sftp.Client {
	sftpClient, err := sftp.NewClient(h.client)
	if err != nil {
		err = h.Reconnect()
		require.NoError(h.context.T(), err)
		sftpClient, err = sftp.NewClient(h.client)
		require.NoError(h.context.T(), err)
	}
	return sftpClient
}

func buildCommandFactory(osFamily oscomp.Family) buildCommandFn {
	if osFamily == oscomp.WindowsFamily {
		return buildCommandOnWindows
	}
	return buildCommandOnLinuxAndMacOS
}

func buildCommandOnWindows(h *Host, command string, envVar EnvVar) string {
	cmd := ""
	envVarSave := map[string]string{}
	for envName, envValue := range envVar {
		previousEnvVar, err := h.executeAndReconnectOnError(fmt.Sprintf("$env:%s", envName))
		if err != nil || previousEnvVar == "" {
			previousEnvVar = "null"
		}
		envVarSave[envName] = previousEnvVar

		cmd += fmt.Sprintf("$env:%s='%s'; ", envName, envValue)
	}
	cmd += fmt.Sprintf("%s; ", command)

	for envName := range envVar {
		cmd += fmt.Sprintf("$env:%s='%s'; ", envName, envVarSave[envName])
	}

	return cmd
}

func buildCommandOnLinuxAndMacOS(_ *Host, command string, envVar EnvVar) string {
	cmd := ""
	for envName, envValue := range envVar {
		cmd += fmt.Sprintf("%s='%s' ", envName, envValue)
	}
	cmd += command
	return cmd
}

// convertToForwardSlashOnWindows replaces backslashes in the path with forward slashes for Windows remote hosts.
// The path is unchanged for non-Windows remote hosts.
//
// This is necessary for remote paths because the sftp package only supports forward slashes, regardless of the local OS.
// The Windows SSH implementation does this conversion, too. Though we have an advantage in that we can check the OSFamily.
// https://github.com/PowerShell/openssh-portable/blob/59aba65cf2e2f423c09d12ad825c3b32a11f408f/scp.c#L636-L650
func convertPathSeparatorFactory(osFamily oscomp.Family) convertPathSeparatorFn {
	if osFamily == oscomp.WindowsFamily {
		return func(s string) string {
			return strings.ReplaceAll(s, "\\", "/")
		}
	}
	return func(s string) string {
		return s
	}
}
