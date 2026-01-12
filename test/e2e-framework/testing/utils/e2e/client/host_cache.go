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

func hostArtifactsClientFactory(sshExecutor *sshExecutor, osFlavor oscomp.Flavor, cloudProvider components.CloudProviderIdentifier, architecture oscomp.Architecture) HostArtifactClient {
	archString := ""
	switch architecture {
	case oscomp.AMD64Arch:
		archString = "x86_64"
	case oscomp.ARM64Arch:
		archString = "aarch64"
	}
	switch cloudProvider {
	case components.CloudProviderAWS:
		switch osFlavor {
		case oscomp.Debian, oscomp.Ubuntu:
			return &hostArtifactsClient{
				cli: &unixAWSCLI{
					sshExecutor: sshExecutor,
					archString:  archString,
					pkgManager: &aptPkgManager{
						sshExecutor: sshExecutor,
					},
				},
			}
		case oscomp.AmazonLinux, oscomp.CentOS, oscomp.RedHat, oscomp.RockyLinux, oscomp.Fedora:
			return &hostArtifactsClient{
				cli: &unixAWSCLI{
					sshExecutor: sshExecutor,
					archString:  archString,
					pkgManager: &yumPkgManager{
						sshExecutor: sshExecutor,
					},
				},
			}
		case oscomp.Suse:
			return &hostArtifactsClient{
				cli: &unixAWSCLI{
					sshExecutor: sshExecutor,
					archString:  archString,
					pkgManager: &zypperPkgManager{
						sshExecutor: sshExecutor,
					},
				},
			}
		case oscomp.WindowsServer:
			return &hostArtifactsClient{
				cli: &windowsAWSCLI{
					sshExecutor: sshExecutor,
				},
			}
		default:
			return &unimplementedHostCache{}
		}
	default:
		return &unimplementedHostCache{}
	}
}

type cli interface {
	install() error
	check() bool
	download(path string, destPath string) error
}

type hostArtifactsClient struct {
	cli cli
}

type pkgManager interface {
	install(pkgName string) error
}

type windowsAWSCLI struct {
	sshExecutor *sshExecutor
}

func (c *windowsAWSCLI) install() error {
	_, err := c.sshExecutor.Execute("Start-Process msiexec.exe -Wait -ArgumentList \"/i https://awscli.amazonaws.com/AWSCLIV2.msi /qn /norestart /L*V ./awscli-install.log\" ")
	return err
}

func (c *windowsAWSCLI) check() bool {
	_, err := c.sshExecutor.Execute("& \"c:\\Program Files\\Amazon\\AWSCLIV2\\aws.exe\" --version")
	return err == nil
}

func (c *windowsAWSCLI) download(path string, destPath string) error {
	_, err := c.sshExecutor.Execute(fmt.Sprintf("& \"c:\\Program Files\\Amazon\\AWSCLIV2\\aws.exe\" s3 cp \"%s\" \"%s\"", path, destPath))
	return err
}

type unixAWSCLI struct {
	sshExecutor *sshExecutor
	archString  string
	pkgManager  pkgManager
}

func (c *unixAWSCLI) install() error {
	_, err := c.sshExecutor.Execute(fmt.Sprintf("curl \"https://awscli.amazonaws.com/awscli-exe-linux-%s.zip\" -o \"awscliv2.zip\"", c.archString))
	if err != nil {
		return err
	}
	err = c.pkgManager.install("unzip")
	if err != nil {
		return err
	}
	_, err = c.sshExecutor.Execute("unzip awscliv2.zip")
	if err != nil {
		return err
	}
	_, err = c.sshExecutor.Execute("sudo ./aws/install")
	if err != nil {
		return err
	}
	return nil
}

func (c *unixAWSCLI) check() bool {
	_, err := c.sshExecutor.Execute("aws --version")
	return err == nil
}

func (c *unixAWSCLI) download(path string, destPath string) error {
	_, err := c.sshExecutor.Execute(fmt.Sprintf("aws s3 cp \"%s\" \"%s\"", path, destPath))
	return err
}

func (c *hostArtifactsClient) Get(path string, destPath string) error {
	if !c.cli.check() {
		if err := c.cli.install(); err != nil {
			return err
		}
	}
	return c.cli.download(fmt.Sprintf("%s/%s", cacheBucketURL, path), destPath)
}

type aptPkgManager struct {
	sshExecutor *sshExecutor
}

func (c *aptPkgManager) install(pkgName string) error {
	_, err := c.sshExecutor.Execute("sudo apt-get install -y " + pkgName)
	return err
}

type yumPkgManager struct {
	sshExecutor *sshExecutor
}

func (c *yumPkgManager) install(pkgName string) error {
	_, err := c.sshExecutor.Execute("sudo yum install -y " + pkgName)
	return err
}

type zypperPkgManager struct {
	sshExecutor *sshExecutor
}

func (c *zypperPkgManager) install(pkgName string) error {
	_, err := c.sshExecutor.Execute("sudo zypper install -y " + pkgName)
	return err
}
