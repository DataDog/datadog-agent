// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

const (
	// Waiting for only 10s as we expect remote to be ready when provisioning
	sshRetryInterval = 2 * time.Second
	sshMaxRetries    = 20
)

type buildCommandFn func(command string, envVars EnvVar) string

type convertPathSeparatorFn func(string) string

// HostArtifactClient is a client that can get files from the artifact bucket to the remote host
type HostArtifactClient interface {
	Get(path string, destPath string) error
}

type sshExecutor struct {
	client     *ssh.Client
	privileged *ssh.Client
	context    common.Context

	username             string
	privilegedUsername   string
	host                 string
	privateKey           []byte
	privateKeyPassphrase []byte
	buildCommand         buildCommandFn
	scrubber             *scrubber.Scrubber
}

// A Host client that is connected to an [ssh.Client].
type Host struct {
	*sshExecutor
	HostArtifactClient

	convertPathSeparator convertPathSeparatorFn
	osFamily             oscomp.Family
	// as per the documentation of http.Transport: "Transports should be reused instead of created as needed."
	httpTransport *http.Transport
}

// NewHost creates a new ssh client to connect to a remote host with
// reconnect retry logic
func NewHost(context common.Context, hostOutput remote.HostOutput) (*Host, error) {
	var privateSSHKey []byte

	privateKeyPath, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.StoreKey(hostOutput.CloudProvider+parameters.PrivateKeyPathSuffix), "")
	if err != nil {
		return nil, err
	}
	privateKeyPassword, err := runner.GetProfile().SecretStore().GetWithDefault(parameters.StoreKey(hostOutput.CloudProvider+parameters.PrivateKeyPasswordSuffix), "")
	if err != nil {
		return nil, err
	}

	if privateKeyPath != "" {
		privateSSHKey, err = os.ReadFile(privateKeyPath)
		if err != nil {
			return nil, err
		}
	}

	var privilegedUsername string
	if hostOutput.OSFamily == oscomp.WindowsFamily {
		privilegedUsername = "Administrator"
	} else {
		privilegedUsername = "root"
	}

	sshExecutor := &sshExecutor{
		context:              context,
		username:             hostOutput.Username,
		privilegedUsername:   privilegedUsername,
		host:                 fmt.Sprintf("%s:%d", hostOutput.Address, hostOutput.Port),
		privateKey:           privateSSHKey,
		privateKeyPassphrase: []byte(privateKeyPassword),
		buildCommand:         buildCommandFactory(hostOutput.OSFamily),
		scrubber:             scrubber.NewWithDefaults(),
	}

	hostArtifacts := hostArtifactsClientFactory(sshExecutor, hostOutput.OSFlavor, hostOutput.CloudProvider, hostOutput.Architecture)

	host := &Host{
		HostArtifactClient:   hostArtifacts,
		sshExecutor:          sshExecutor,
		convertPathSeparator: convertPathSeparatorFactory(hostOutput.OSFamily),
		osFamily:             hostOutput.OSFamily,
	}

	host.httpTransport = host.newHTTPTransport()

	err = host.Reconnect()
	return host, err
}

// Reconnect closes the current ssh client and creates a new one, with retries.
func (h *sshExecutor) Reconnect() error {
	utils.Logf(h.context.T(), "Reconnecting to host")
	if h.client != nil {
		_ = h.client.Close()
	}
	if h.privileged != nil {
		_ = h.privileged.Close()
	}
	_, err := backoff.Retry(context.Background(), func() (any, error) {
		client, err := getSSHClient(h.username, h.host, h.privateKey, h.privateKeyPassphrase)
		if err != nil {
			return nil, err
		}
		h.client = client

		privileged, err := getSSHClient(h.privilegedUsername, h.host, h.privateKey, h.privateKeyPassphrase)
		if err != nil {
			utils.Logf(h.context.T(), "Unable to create privileged SSH connection: %v", err)
			// Ignore this error for now, since SSH connection as root are not enable on some providers
		}
		h.privileged = privileged
		return nil, nil
	}, backoff.WithBackOff(backoff.NewConstantBackOff(sshRetryInterval)), backoff.WithMaxTries(sshMaxRetries))
	return err
}

// Execute executes a command and returns an error if any.
func (h *sshExecutor) Execute(command string, options ...ExecuteOption) (string, error) {
	params, err := optional.MakeParams(options...)
	if err != nil {
		return "", err
	}
	command = h.buildCommand(command, params.EnvVariables)
	return h.executeAndReconnectOnError(command)
}

func (h *sshExecutor) executeAndReconnectOnError(command string) (string, error) {
	scrubbedCommand := h.scrubber.ScrubLine(command) // scrub the command in case it contains secrets
	utils.Logf(h.context.T(), "Executing command `%s`", scrubbedCommand)
	stdout, err := execute(h.client, command)
	if err != nil && strings.Contains(err.Error(), "failed to create session:") {
		err = h.Reconnect()
		if err != nil {
			return "", err
		}
		stdout, err = execute(h.client, command)
	}
	if err != nil {
		return "", fmt.Errorf("%v: %w", stdout, err)
	}
	return stdout, err
}

// Start a command and returns session, and an error if any.
func (h *sshExecutor) Start(command string, options ...ExecuteOption) (*ssh.Session, io.WriteCloser, io.Reader, error) {
	params, err := optional.MakeParams(options...)
	if err != nil {
		return nil, nil, nil, err
	}
	command = h.buildCommand(command, params.EnvVariables)
	return h.startAndReconnectOnError(command)
}

func (h *sshExecutor) startAndReconnectOnError(command string) (*ssh.Session, io.WriteCloser, io.Reader, error) {
	scrubbedCommand := h.scrubber.ScrubLine(command) // scrub the command in case it contains secrets
	utils.Logf(h.context.T(), "Executing command `%s`", scrubbedCommand)
	session, stdin, stdout, err := start(h.client, command)
	if err != nil && strings.Contains(err.Error(), "failed to create session:") {
		err = h.Reconnect()
		if err != nil {
			return nil, nil, nil, err
		}
		session, stdin, stdout, err = start(h.client, command)
	}
	return session, stdin, stdout, err
}

// mustExecuteCommand executes a command over SSH and returns the stdout output.
// It is a package-level function so that it can be instrumented by Orchestrion via //dd:span.
//
//dd:span command:command
func mustExecuteCommand(executor *sshExecutor, command string, options ...ExecuteOption) (string, error) {
	return executor.Execute(command, options...)
}

// MustExecute executes a command and requires no error.
func (h *sshExecutor) MustExecute(command string, options ...ExecuteOption) string {
	stdout, err := mustExecuteCommand(h, command, options...)
	require.NoError(h.context.T(), err)
	return stdout
}

// CopyFileFromFS creates a sftp session and copy a single embedded file to the remote host through SSH
func (h *Host) CopyFileFromFS(fs fs.FS, src, dst string) {
	utils.Logf(h.context.T(), "Copying file from local %s to remote %s", src, dst)
	dst = h.convertPathSeparator(dst)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()
	file, err := fs.Open(src)
	require.NoError(h.context.T(), err)
	defer file.Close()
	err = copyFileFromIoReader(sftpClient, file, dst)
	require.NoError(h.context.T(), err)
}

// CopyFile creates a sftp session and copy a single file to the remote host through SSH
func (h *Host) CopyFile(src string, dst string) {
	utils.Logf(h.context.T(), "Copying file from local %s to remote %s", src, dst)
	dst = h.convertPathSeparator(dst)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()
	err := copyFile(sftpClient, src, dst)
	require.NoError(h.context.T(), err)
}

// CopyFolder create a sftp session and copy a folder to remote host through SSH
func (h *Host) CopyFolder(srcFolder string, dstFolder string) error {
	utils.Logf(h.context.T(), "Copying folder from local %s to remote %s", srcFolder, dstFolder)
	dstFolder = h.convertPathSeparator(dstFolder)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()
	return copyFolder(sftpClient, srcFolder, dstFolder)
}

// FileExists create a sftp session to and returns true if the file exists and is a regular file
func (h *Host) FileExists(path string) (bool, error) {
	utils.Logf(h.context.T(), "Checking if file exists: %s", path)
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

// EnsureFileIsReadable add readable rights to a remote file
func (h *Host) EnsureFileIsReadable(path string) error {
	// ensure the file is readable on the remote host
	if h.osFamily != oscomp.WindowsFamily {
		_, err := h.Execute("sudo chmod +r " + path)
		if err != nil {
			return fmt.Errorf("failed to make file readable: %w", err)
		}
	}
	return nil
}

// GetFile create a sftp session and copy a single file from the remote host through SSH
func (h *Host) GetFile(src string, dst string) error {
	utils.Logf(h.context.T(), "Copying file from remote %s to local %s", src, dst)
	dst = h.convertPathSeparator(dst)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()
	return downloadFile(sftpClient, src, dst)
}

// GetFolder create a sftp session and copy a folder from the remote host through SSH
func (h *Host) GetFolder(srcFolder string, dstFolder string) error {
	utils.Logf(h.context.T(), "Copying folder from remote %s to local %s", srcFolder, dstFolder)
	srcFolder = h.convertPathSeparator(srcFolder)
	dstFolder = h.convertPathSeparator(dstFolder)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()
	return downloadFolder(sftpClient, srcFolder, dstFolder)
}

func (h *Host) readFileWithClient(sftpClient *sftp.Client, path string) ([]byte, error) {
	utils.Logf(h.context.T(), "Reading file with client at %s", path)
	path = h.convertPathSeparator(path)
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

// ReadFile reads the content of the file, return bytes read and error if any
func (h *Host) ReadFile(path string) ([]byte, error) {
	utils.Logf(h.context.T(), "Reading file at %s", path)
	return h.readFileWithClient(h.getSFTPClient(), path)
}

// ReadFilePrivileged reads the content of the file with a privileged user, return bytes read and error if any
func (h *Host) ReadFilePrivileged(path string) ([]byte, error) {
	utils.Logf(h.context.T(), "Reading file with privileges at %s", path)
	return h.readFileWithClient(h.getSFTPPrivilegedClient(), path)
}

// WriteFile write content to the file and returns the number of bytes written and error if any
func (h *Host) WriteFile(path string, content []byte) (int64, error) {
	utils.Logf(h.context.T(), "Writing to file at %s", path)
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
	utils.Logf(h.context.T(), "Appending to file at %s", path)
	path = h.convertPathSeparator(path)
	if os == "linux" {
		return h.appendWithSudo(path, content)
	}
	return h.appendWithSftp(path, content)
}

// ReadDir returns list of directory entries in path
func (h *Host) ReadDir(path string) ([]fs.DirEntry, error) {
	utils.Logf(h.context.T(), "Reading filesystem at %s", path)
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

// FindFiles returns a list of files with a given name
func (h *Host) FindFiles(name string) ([]string, error) {
	h.context.T().Logf("Finding files with name %s", name)
	switch h.osFamily {
	case oscomp.WindowsFamily:
		out, err := h.Execute("Get-ChildItem -Path C:\\ -Filter " + name)
		if err != nil {
			return nil, err
		}
		return strings.Split(out, "\n"), nil
	case oscomp.LinuxFamily:
		out, err := h.Execute("sudo find / -name " + name)
		if err != nil {
			return nil, err
		}
		return strings.Split(out, "\n"), nil
	default:
		return nil, errors.ErrUnsupported
	}
}

// Lstat returns a FileInfo structure describing path.
// if path is a symbolic link, the FileInfo structure describes the symbolic link.
func (h *Host) Lstat(path string) (fs.FileInfo, error) {
	utils.Logf(h.context.T(), "Reading file info of %s", path)
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()

	return sftpClient.Lstat(path)
}

// MkdirAll creates the specified directory along with any necessary parents.
// If the path is already a directory, does nothing and returns nil.
// Otherwise returns an error if any.
func (h *Host) MkdirAll(path string) error {
	utils.Logf(h.context.T(), "Creating directory %s", path)
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()

	return sftpClient.MkdirAll(path)
}

// Remove removes the specified file or directory.
// Returns an error if file or directory does not exist, or if the directory is not empty.
func (h *Host) Remove(path string) error {
	utils.Logf(h.context.T(), "Removing %s", path)
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()

	return sftpClient.Remove(path)
}

// RemoveAll recursively removes all files/folders in the specified directory.
// Returns an error if the directory does not exist.
func (h *Host) RemoveAll(path string) error {
	utils.Logf(h.context.T(), "Removing all under %s", path)
	path = h.convertPathSeparator(path)
	sftpClient := h.getSFTPClient()
	defer sftpClient.Close()

	return sftpClient.RemoveAll(path)
}

// DialPort creates a connection from the remote host to its `port`.
func (h *Host) DialPort(port uint16) (net.Conn, error) {
	utils.Logf(h.context.T(), "Creating connection to host port %d", port)
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
		out, err := h.Execute("echo $env:TEMP")
		if err != nil {
			return out, err
		}
		return strings.TrimSpace(out), nil
	case oscomp.LinuxFamily:
		return "/tmp", nil
	default:
		return "", errors.ErrUnsupported
	}
}

// GetAgentConfigFolder returns the agent config folder path for the host
func (h *Host) GetAgentConfigFolder() (string, error) {
	switch h.osFamily {
	case oscomp.WindowsFamily:
		out, err := h.Execute("echo $env:PROGRAMDATA")
		if err != nil {
			return out, err
		}
		return strings.TrimSpace(out) + "\\Datadog", nil
	case oscomp.LinuxFamily:
		return "/etc/datadog-agent", nil
	case oscomp.MacOSFamily:
		return "/opt/datadog-agent/etc", nil
	default:
		return "", errors.ErrUnsupported
	}
}

// GetLogsFolder returns the logs folder path for the host
func (h *Host) GetLogsFolder() (string, error) {
	switch h.osFamily {
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

// JoinPath joins the path elements with the correct separator for the host
func (h *Host) JoinPath(path ...string) string {
	switch h.osFamily {
	case oscomp.WindowsFamily:
		return strings.Join(path, "\\")
	default:
		return strings.Join(path, "/")
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
	sftpClient, err := sftp.NewClient(h.client, sftp.UseConcurrentWrites(true))
	if err != nil {
		err = h.Reconnect()
		require.NoError(h.context.T(), err)
		sftpClient, err = sftp.NewClient(h.client, sftp.UseConcurrentWrites(true))
		require.NoError(h.context.T(), err)
	}
	return sftpClient
}

func (h *Host) getSFTPPrivilegedClient() *sftp.Client {
	if h.privileged == nil {
		// Some cloud provider don't provide SSH connection as root (GCP) required for these file operations
		utils.Logf(h.context.T(), "Can't SFTP files without a privileged SSH connection")
		h.context.T().Fail()
		return nil
	}
	sftpClient, err := sftp.NewClient(h.privileged, sftp.UseConcurrentWrites(true))
	if err != nil {
		err = h.Reconnect()
		require.NoError(h.context.T(), err)
		sftpClient, err = sftp.NewClient(h.privileged, sftp.UseConcurrentWrites(true))
		require.NoError(h.context.T(), err)
	}
	return sftpClient
}

// HTTPTransport returns an http.RoundTripper which dials the remote host.
// This transport can only reach the host.
func (h *Host) HTTPTransport() http.RoundTripper {
	return h.httpTransport
}

// NewHTTPClient returns an *http.Client which dials the remote host.
// This client can only reach the host.
func (h *Host) NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: h.httpTransport,
	}
}

func (h *Host) newHTTPTransport() *http.Transport {
	return &http.Transport{
		DialContext: func(_ context.Context, _, addr string) (net.Conn, error) {
			hostname, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// best effort to detect logic errors around the hostname
			// if the hostname provided to dial is not one of those, return an error as
			// it's likely an incorrect use of this transport
			validHostnames := map[string]struct{}{
				"":                             {},
				"localhost":                    {},
				"127.0.0.1":                    {},
				h.client.RemoteAddr().String(): {},
			}

			if _, ok := validHostnames[hostname]; !ok {
				return nil, fmt.Errorf("request hostname %s does not match any valid host name", hostname)
			}

			portInt, err := strconv.Atoi(port)
			if err != nil {
				return nil, err
			}
			return h.DialPort(uint16(portInt))
		},
		// skip verify like we do when reaching out to the agent
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		// from http.DefaultTransport
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func buildCommandFactory(osFamily oscomp.Family) buildCommandFn {
	if osFamily == oscomp.WindowsFamily {
		return buildCommandOnWindows
	}
	return buildCommandOnLinuxAndMacOS
}

func buildCommandOnWindows(command string, envVar EnvVar) string {
	var builder strings.Builder

	// Set $ErrorActionPreference to 'Stop' to cause PowerShell to stop on an error instead
	// of the default 'Continue' behavior.
	// This also ensures that Execute() will return an error when a command fails.
	// Note that this only applies to PowerShell commands, not to external commands or native binaries.
	//
	// For example, if the command is (Get-Service -Name ddnpm).Status and the service does not exist,
	// then by default the command will print an error but the exit code will be 0 and Execute() will not return an error.
	// By setting $ErrorActionPreference to 'Stop', Execute() will return an error as one would expect.
	//
	// Thus, we default to 'Stop' to make sure that an error is raised when the command fails instead of failing silently.
	// Commands that this causes issues for will be immediately noticed and can be adjusted as needed, instead of
	// silent errors going unnoticed and affecting test results.
	//
	// To ignore errors, prefix command with $ErrorActionPreference='Continue' or use -ErrorAction Continue
	// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_preference_variables#erroractionpreference
	builder.WriteString("$ErrorActionPreference='Stop'; ")

	for envName, envValue := range envVar {
		fmt.Fprintf(&builder, "$env:%s='%s'; ", envName, envValue)
	}
	// By default, powershell will just exit with 0 or 1, so we call exit to preserve
	// the exit code of the command provided by the caller.
	// The caller's command may not modify LASTEXITCODE, so manually reset it first,
	// then only call exit if the command provided by the caller fails.
	//
	// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_automatic_variables?#lastexitcode
	// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_powershell_exe?#-command
	fmt.Fprintf(&builder, "$LASTEXITCODE=0; %s; if (-not $?) { exit $LASTEXITCODE }", command)
	// NOTE: Do not add more commands after the command provided by the caller.
	//
	// `$ErrorActionPreference`='Stop' only applies to PowerShell commands, not to
	// external commands or native binaries, thus later commands will still be executed.
	// Additional commands will overwrite the exit code of the command provided by
	// the caller which may cause errors to be missed/ignored.
	// If it becomes necessary to run more commands after the command provided by the
	// caller, we will need to find a way to ensure that the exit code of the command
	// provided by the caller is preserved.

	return builder.String()
}

func buildCommandOnLinuxAndMacOS(command string, envVar EnvVar) string {
	var builder strings.Builder
	for envName, envValue := range envVar {
		fmt.Fprintf(&builder, "%s='%s' ", envName, envValue)
	}
	builder.WriteString(command)
	return builder.String()
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
