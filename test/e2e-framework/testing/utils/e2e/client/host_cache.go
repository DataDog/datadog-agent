// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

const (
	cacheBucketURL = "s3://agent-e2e-s3-bucket"
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
	_, err := c.sshExecutor.Execute("Start-Process msiexec.exe -Wait -ArgumentList \"/i https://awscli.amazonaws.com/AWSCLIV2.msi /qn /norestart /L*V ./awscli-install.log\" ")
	return err
}

func (c *windowsAWSCLI) download(path string, destPath string) error {
	if err := c.ensureInstalled(); err != nil {
		return err
	}
	_, err := c.sshExecutor.Execute(fmt.Sprintf("& \"c:\\Program Files\\Amazon\\AWSCLIV2\\aws.exe\" s3 cp \"%s\" \"%s\"", path, destPath))
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
