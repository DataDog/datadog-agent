// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/pkg/sftp"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

const (
	cacheBucketURL = "s3://agent-e2e-s3-bucket"

	// awsCliInstallRetries/awsS3CopyRetries account for a flaky AWS network on the test host.
	awsCliInstallRetries = 3
	awsS3CopyRetries     = 3
	awsRetryInterval     = 5 * time.Second

	awsCliRemoteInstallLogPath = `C:\Windows\Temp\awscli-install.log`

	// msiExitSuccessRebootRequired is the successful msiexec exit code returned when /norestart
	// defers a pending reboot. See https://learn.microsoft.com/en-us/windows/win32/msi/error-codes
	msiExitSuccessRebootRequired = 3010
)

type unimplementedHostCache struct{}

func (c *unimplementedHostCache) Get(_ string, _ string) error {
	return errors.New("not implemented")
}

func hostArtifactsClientFactory(sshExecutor *sshExecutor, osFlavor oscomp.Flavor, cloudProvider components.CloudProviderIdentifier, _ oscomp.Architecture) HostArtifactClient {
	switch cloudProvider {
	case components.CloudProviderAWS:
		switch osFlavor {
		case oscomp.Debian, oscomp.Ubuntu, oscomp.AmazonLinux, oscomp.CentOS, oscomp.RedHat, oscomp.RockyLinux, oscomp.Fedora, oscomp.Suse, oscomp.AlmaLinux:
			return &hostArtifactsClient{
				cli: &unixAWSCLI{sshExecutor: sshExecutor},
			}
		case oscomp.WindowsServer:
			return &hostArtifactsClient{
				cli: &windowsAWSCLI{sshExecutor: sshExecutor},
			}
		default:
			return &unimplementedHostCache{}
		}
	default:
		return &unimplementedHostCache{}
	}
}

type cli interface {
	download(path string, destPath string) error
}

type hostArtifactsClient struct {
	cli cli
}

type windowsAWSCLI struct {
	sshExecutor *sshExecutor
}

// ensureInstalled installs AWS CLI v2 if it isn't already present on the host.
//
// TEMPORARY: Linux e2e AMIs have AWS CLI v2 pre-baked, but the Windows Server
// AMIs we use have not yet been re-baked with it. Once ami-builder ships
// Windows e2e AMI variants with AWS CLI pre-installed, delete this method and
// the call from download() below. Tracked in ACIX-1305.
func (c *windowsAWSCLI) ensureInstalled() error {
	if _, err := c.sshExecutor.Execute("& \"c:\\Program Files\\Amazon\\AWSCLIV2\\aws.exe\" --version"); err == nil {
		return nil
	}
	_, err := backoff.Retry(context.Background(), func() (any, error) {
		// Start-Process's own exit code only reflects whether it launched msiexec, not whether the
		// install succeeded, so capture the process with -PassThru and check its ExitCode explicitly.
		_, err := c.sshExecutor.Execute(fmt.Sprintf(
			`$p = Start-Process msiexec.exe -Wait -PassThru -ArgumentList "/i https://awscli.amazonaws.com/AWSCLIV2.msi /qn /norestart /L*V %s"; if ($p.ExitCode -ne 0 -and $p.ExitCode -ne %d) { throw "msiexec exited with code $($p.ExitCode)" }`,
			awsCliRemoteInstallLogPath, msiExitSuccessRebootRequired))
		return nil, err
	}, backoff.WithBackOff(backoff.NewConstantBackOff(awsRetryInterval)), backoff.WithMaxTries(awsCliInstallRetries))
	c.collectInstallLog()
	return err
}

// collectInstallLog best-effort copies the AWS CLI msiexec log to the test's artifacts folder,
// mirroring the log collection test/new-e2e/tests/windows/common/msi.go does for Agent MSI installs.
func (c *windowsAWSCLI) collectInstallLog() {
	sftpClient, err := sftp.NewClient(c.sshExecutor.client, sftp.UseConcurrentWrites(true))
	if err != nil {
		c.sshExecutor.context.Logf("failed to collect AWS CLI install log: %v", err)
		return
	}
	defer sftpClient.Close()

	remotePath := strings.ReplaceAll(awsCliRemoteInstallLogPath, "\\", "/")
	localPath := filepath.Join(c.sshExecutor.context.SessionOutputDir(), "awscli-install.log")
	if err := downloadFile(sftpClient, remotePath, localPath); err != nil {
		c.sshExecutor.context.Logf("failed to collect AWS CLI install log: %v", err)
	}
}

func (c *windowsAWSCLI) download(path string, destPath string) error {
	if err := c.ensureInstalled(); err != nil {
		return err
	}
	_, err := backoff.Retry(context.Background(), func() (any, error) {
		_, err := c.sshExecutor.Execute(fmt.Sprintf("& \"c:\\Program Files\\Amazon\\AWSCLIV2\\aws.exe\" s3 cp \"%s\" \"%s\"", path, destPath))
		return nil, err
	}, backoff.WithBackOff(backoff.NewConstantBackOff(awsRetryInterval)), backoff.WithMaxTries(awsS3CopyRetries))
	return err
}

type unixAWSCLI struct {
	sshExecutor *sshExecutor
}

func (c *unixAWSCLI) download(path string, destPath string) error {
	_, err := c.sshExecutor.Execute(fmt.Sprintf("aws s3 cp \"%s\" \"%s\"", path, destPath))
	return err
}

func (c *hostArtifactsClient) Get(path string, destPath string) error {
	return c.cli.download(fmt.Sprintf("%s/%s", cacheBucketURL, path), destPath)
}
