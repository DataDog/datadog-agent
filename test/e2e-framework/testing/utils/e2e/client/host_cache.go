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
				},
			}
		case oscomp.AmazonLinux, oscomp.CentOS, oscomp.RedHat, oscomp.RockyLinux, oscomp.Fedora:
			return &hostArtifactsClient{
				cli: &unixAWSCLI{
					sshExecutor: sshExecutor,
					archString:  archString,
				},
			}
		case oscomp.Suse:
			return &hostArtifactsClient{
				cli: &unixAWSCLI{
					sshExecutor: sshExecutor,
					archString:  archString,
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
	download(path string, destPath string) error
}

type hostArtifactsClient struct {
	cli cli
}

type windowsAWSCLI struct {
	sshExecutor *sshExecutor
}

func (c *windowsAWSCLI) download(path string, destPath string) error {
	_, err := c.sshExecutor.Execute(fmt.Sprintf("& \"c:\\Program Files\\Amazon\\AWSCLIV2\\aws.exe\" s3 cp \"%s\" \"%s\"", path, destPath))
	return err
}

type unixAWSCLI struct {
	sshExecutor *sshExecutor
	archString  string
}

func (c *unixAWSCLI) download(path string, destPath string) error {
	_, err := c.sshExecutor.Execute(fmt.Sprintf("aws s3 cp \"%s\" \"%s\"", path, destPath))
	return err
}

func (c *hostArtifactsClient) Get(path string, destPath string) error {
	return c.cli.download(fmt.Sprintf("%s/%s", cacheBucketURL, path), destPath)
}
