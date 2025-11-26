// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2"
)

type amiInformation struct {
	id          string
	defaultUser string
	readyFunc   command.ReadyFunc
}

var defaultUsers = map[os.Flavor]string{
	os.WindowsServer:  "Administrator",
	os.Ubuntu:         "ubuntu",
	os.AmazonLinux:    "ec2-user",
	os.AmazonLinuxECS: "ec2-user",
	os.Debian:         "admin",
	os.RedHat:         "ec2-user",
	os.Suse:           "ec2-user",
	os.Fedora:         "fedora",
	os.CentOS:         "centos",
	os.RockyLinux:     "cloud-user",
	os.MacosOS:        "ec2-user",
}

type amiResolverFunc func(aws.Environment, *os.Descriptor) (string, error)

var amiResolvers = map[os.Flavor]amiResolverFunc{
	os.WindowsServer:  resolveWindowsAMI,
	os.Ubuntu:         resolveUbuntuAMI,
	os.AmazonLinux:    resolveAmazonLinuxAMI,
	os.AmazonLinuxECS: resolveAmazonLinuxECSAMI,
	os.Debian:         resolveDebianAMI,
	os.RedHat:         resolveRedHatAMI,
	os.Suse:           resolveSuseAMI,
	os.Fedora:         resolveFedoraAMI,
	os.CentOS:         resolveCentOSAMI,
	os.RockyLinux:     resolveRockyLinuxAMI,
	os.MacosOS:        resolveMacosAMI,
}

// Returns the default version for the given flavor
func getDefaultVersion(flavor os.Flavor) (string, error) {
	if version, ok := os.LinuxDescriptorsDefault[flavor]; ok {
		return version.Version, nil
	}
	if version, ok := os.WindowsDescriptorsDefault[flavor]; ok {
		return version.Version, nil
	}
	if version, ok := os.MacOSDescriptorsDefault[flavor]; ok {
		return version.Version, nil
	}

	return "", fmt.Errorf("no default version found for flavor %s, this flavor should be added to the default descriptors", flavor)
}

// resolveOS returns the AMI ID for the given OS.
// Note that you may get this error in some cases:
// OptInRequired: In order to use this AWS Marketplace product you need to accept terms and subscribe
// This means that you need to go to the AWS Marketplace and accept the terms of the AMI.
func resolveOS(e aws.Environment, vmArgs *vmArgs) (*amiInformation, error) {
	if vmArgs.ami == "" {
		var err error

		// If no AMI set and latest AMI is requested, resolve the AMI
		if vmArgs.osInfo.Version == "" && vmArgs.useLatestAMI {
			vmArgs.ami, err = amiResolvers[vmArgs.osInfo.Flavor](e, vmArgs.osInfo)
			if err != nil {
				return nil, err
			}
		} else {
			// If the version is not set, use the default version of this flavor
			if vmArgs.osInfo.Version == "" {
				vmArgs.osInfo.Version, err = getDefaultVersion(vmArgs.osInfo.Flavor)
				if err != nil {
					return nil, err
				}
			}
			vmArgs.ami, err = aws.GetAMI(vmArgs.osInfo)
			if err != nil {
				return nil, err
			}
		}
	}
	fmt.Printf("Using AMI %s\n for stack %s\n", vmArgs.ami, e.Ctx().Stack())

	amiInfo := &amiInformation{
		id:          vmArgs.ami,
		defaultUser: defaultUsers[vmArgs.osInfo.Flavor],
	}

	switch vmArgs.osInfo.Family() { // nolint:exhaustive
	case os.LinuxFamily:
		if vmArgs.osInfo.Version == os.AmazonLinux2018.Version && vmArgs.osInfo.Flavor == os.AmazonLinux2018.Flavor {
			amiInfo.readyFunc = command.WaitForSuccessfulConnection
		} else {
			amiInfo.readyFunc = command.WaitForCloudInit
		}
	case os.WindowsFamily, os.MacOSFamily:
		amiInfo.readyFunc = command.WaitForSuccessfulConnection
	default:
		return nil, fmt.Errorf("unsupported OS family %v", vmArgs.osInfo.Family())
	}

	return amiInfo, nil
}

func warnOSNotUsingLatestAMI(e aws.Environment, osInfo *os.Descriptor) {
	e.Ctx().Log.Warn(fmt.Sprintf("%s is not using the latest AMI but a hardcoded one", osInfo.Flavor.String()), nil)
}

func resolveWindowsAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	if osInfo.Architecture == os.ARM64Arch {
		return "", errors.New("ARM64 is not supported for Windows")
	}
	if osInfo.Version == "" {
		osInfo.Version = os.WindowsServerDefault.Version
	}

	return ec2.GetAMIFromSSM(e, fmt.Sprintf("/aws/service/ami-windows-latest/Windows_Server-%s-English-Full-Base", osInfo.Version))
}

func resolveAmazonLinuxAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	var paramName string
	switch osInfo.Version {
	case "", os.AmazonLinuxECS2.Version:
		paramName = fmt.Sprintf("amzn2-ami-hvm-%s-gp2", osInfo.Architecture)
	case os.AmazonLinuxECS2023.Version:
		paramName = fmt.Sprintf("al2023-ami-kernel-default-%s", osInfo.Architecture)
	case os.AmazonLinux2018.Version:
		if osInfo.Architecture != os.AMD64Arch {
			return "", fmt.Errorf("arch %s is not supported for Amazon Linux 2018", osInfo.Architecture)
		}
		return ec2.SearchAMI(e, "669783387624", "amzn-ami-2018.03.*-amazon-ecs-optimized", string(osInfo.Architecture))
	default:
		return "", fmt.Errorf("unsupported Amazon Linux version %s", osInfo.Version)
	}

	return ec2.GetAMIFromSSM(e, fmt.Sprintf("/aws/service/ami-amazon-linux-latest/%s", paramName))
}

func resolveAmazonLinuxECSAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	var paramName string
	switch osInfo.Version {
	case "", os.AmazonLinuxECSDefault.Version:
		paramName = "amazon-linux-2"
	case os.AmazonLinuxECS2023.Version:
		paramName = "amazon-linux-2023"
	default:
		return "", fmt.Errorf("unsupported Amazon Linux ECS version %s", osInfo.Version)
	}

	if osInfo.Architecture == os.ARM64Arch {
		paramName += "/arm64"
	}

	return ec2.GetAMIFromSSM(e, fmt.Sprintf("/aws/service/ecs/optimized-ami/%s/recommended/image_id", paramName))
}

func resolveUbuntuAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	if osInfo.Version == "" {
		osInfo.Version = os.UbuntuDefault.Version
	}
	volumeType := "ebs-gp2"

	paramArch := osInfo.Architecture
	if paramArch == os.AMD64Arch {
		// Override required as the architecture is x86_64 but the SSM parameter is amd64
		paramArch = "amd64"
	}
	if osInfo.Version == "24-04" {
		volumeType = "ebs-gp3"
	}

	return ec2.GetAMIFromSSM(e, fmt.Sprintf("/aws/service/canonical/ubuntu/server/%s/stable/current/%s/hvm/%s/ami-id", osInfo.Version, paramArch, volumeType))
}

func resolveDebianAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	if osInfo.Version == "" {
		osInfo.Version = os.DebianDefault.Version
	}

	paramArch := osInfo.Architecture
	if paramArch == os.AMD64Arch {
		// Override required as the architecture is x86_64 but the SSM parameter is amd64
		paramArch = "amd64"
	}

	return ec2.GetAMIFromSSM(e, fmt.Sprintf("/aws/service/debian/release/%s/latest/%s", osInfo.Version, paramArch))
}

func resolveRedHatAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	if osInfo.Version == "" {
		osInfo.Version = os.RedHatDefault.Version
	}

	// Use recommended name query filter by RedHat https://access.redhat.com/solutions/15356
	redhatOwner := "309956199498"
	return ec2.SearchAMI(e, redhatOwner, fmt.Sprintf("RHEL-%s*", osInfo.Version), string(osInfo.Architecture))
}

func resolveSuseAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	if osInfo.Version == "" {
		osInfo.Version = os.SuseDefault.Version
	}

	if osInfo.Version == "15-4" {
		warnOSNotUsingLatestAMI(e, osInfo)
		if osInfo.Architecture == os.AMD64Arch {
			return "ami-067dfda331f8296b0", nil // Private copy of the AMI dd-agent-sles-15-x86_64
		} else if osInfo.Architecture == os.ARM64Arch {
			return "ami-08350d1d1649d8c05", nil
		}
		return "", fmt.Errorf("architecture %s is not supported for SUSE %s", osInfo.Architecture, osInfo.Version)
	}

	return ec2.GetAMIFromSSM(e, fmt.Sprintf("/aws/service/suse/sles/%s/%s/latest", osInfo.Version, osInfo.Architecture))
}

func resolveFedoraAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	if osInfo.Architecture == os.ARM64Arch {
		return "", errors.New("ARM64 is not supported for Fedora")
	}

	if osInfo.Version == "" {
		osInfo.Version = os.FedoraDefault.Version
	}

	return ec2.SearchAMI(e, "125523088429", fmt.Sprintf("Fedora-Cloud-Base*-%s-*", osInfo.Version), string(osInfo.Architecture))
}

func resolveCentOSAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	if osInfo.Version == "" {
		osInfo.Version = os.CentOSDefault.Version
	}

	if osInfo.Architecture == os.ARM64Arch {
		if osInfo.Version == "7" {
			warnOSNotUsingLatestAMI(e, osInfo)
			return "ami-0cb7a00afccf30559", nil
		}
		return "", fmt.Errorf("ARM64 is not supported for CentOS %s", osInfo.Version)
	}

	if osInfo.Version == "7" {
		warnOSNotUsingLatestAMI(e, osInfo)
		return "ami-036de472bb001ae9c", nil
	}

	return ec2.SearchAMI(e, "679593333241", fmt.Sprintf("CentOS-%s-*-*.x86_64*", osInfo.Version), string(osInfo.Architecture))
}

func resolveRockyLinuxAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	warnOSNotUsingLatestAMI(e, osInfo)

	if osInfo.Version != "" {
		return "", fmt.Errorf("cannot set version for Rocky Linux")
	}

	var amiID string
	switch osInfo.Architecture {
	case os.AMD64Arch:
		amiID = "ami-071db23a8a6271e2c"
	case os.ARM64Arch:
		amiID = "ami-0a22577ee769ab5b0"
	default:
		return "", fmt.Errorf("architecture %s is not supported for Rocky Linux", osInfo.Architecture)
	}

	return amiID, nil
}

func resolveMacosAMI(e aws.Environment, osInfo *os.Descriptor) (string, error) {
	if osInfo.Version == "" {
		osInfo.Version = os.MacOSSonoma.Version
	}

	return ec2.GetAMIFromSSM(e, fmt.Sprintf("/aws/service/ec2-macos/%s/%s_mac/latest/image_id", osInfo.Version, osInfo.Architecture))
}
