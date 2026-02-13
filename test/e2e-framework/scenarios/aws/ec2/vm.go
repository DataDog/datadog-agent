// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2"

	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// NewVM creates an EC2 Instance and returns a Remote component.
// Without any parameter it creates an Ubuntu VM on AMD64 architecture.
func NewVM(e aws.Environment, name string, params ...VMOption) (*remote.Host, error) {
	vmArgs, err := buildArgs(params...)
	if err != nil {
		return nil, err
	}

	// Default missing parameters
	if err = defaultVMArgs(e, vmArgs); err != nil {
		return nil, err
	}

	// Resolve AMI if necessary
	amiInfo, err := resolveOS(e, vmArgs)
	if err != nil {
		return nil, err
	}
	sshUser := amiInfo.defaultUser
	if infraSSHUser := e.InfraSSHUser(); infraSSHUser != "" {
		sshUser = infraSSHUser
	}

	// Create the EC2 instance
	return components.NewComponent(&e, e.Namer.ResourceName(name), func(c *remote.Host) error {
		opts := []pulumi.ResourceOption{pulumi.Parent(c)}
		opts = append(opts, vmArgs.pulumiResourceOptions...)
		c.CloudProvider = pulumi.String(components.CloudProviderAWS).ToStringOutput()

		instanceArgs := ec2.InstanceArgs{
			AMI:                amiInfo.id,
			InstanceType:       vmArgs.instanceType,
			UserData:           vmArgs.userData,
			InstanceProfile:    vmArgs.instanceProfile,
			HTTPTokensRequired: vmArgs.httpTokensRequired,
			Tenancy:            vmArgs.tenancy,
			HostID:             pulumi.String(vmArgs.hostID),
			VolumeThroughput:   vmArgs.volumeThroughput,
		}

		if vmArgs.osInfo.Family() == os.MacOSFamily && vmArgs.hostID == "" {
			dedicatedHost, err := ec2.NewDedicatedHost(e, name, ec2.DedicatedHostArgs{
				InstanceType: vmArgs.instanceType,
			})
			if err != nil {
				return err
			}
			instanceArgs.HostID = dedicatedHost.Arn.ApplyT(func(arn string) pulumi.StringInput {
				splitted := strings.Split(arn, "/")
				return pulumi.String(splitted[len(splitted)-1])
			}).(pulumi.StringInput)
		}

		// Create the EC2 instance
		instance, err := ec2.NewInstance(e, name, instanceArgs, opts...)
		if err != nil {
			return err
		}

		// Create connection
		conn, err := remote.NewConnection(
			instance.PrivateIp,
			sshUser,
			remote.WithPrivateKeyPath(e.DefaultPrivateKeyPath()),
			remote.WithPrivateKeyPassword(e.DefaultPrivateKeyPassword()),
		)
		if err != nil {
			return err
		}

		err = remote.InitHost(&e, conn.ToConnectionOutput(), *vmArgs.osInfo, sshUser, pulumi.String("").ToStringOutput(), amiInfo.readyFunc, c)

		if err != nil {
			return err
		}

		// reset the windows password on Windows
		if vmArgs.osInfo.Family() == os.WindowsFamily {
			// The password contains characters from three of the following categories:
			// 		* Uppercase letters of European languages (A through Z, with diacritic marks, Greek and Cyrillic characters).
			// 		* Lowercase letters of European languages (a through z, sharp-s, with diacritic marks, Greek and Cyrillic characters).
			// 		* Base 10 digits (0 through 9).
			// 		* Non-alphanumeric characters (special characters): '-!"#$%&()*,./:;?@[]^_`{|}~+<=>
			// Source: https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-10/security/threat-protection/security-policy-settings/password-must-meet-complexity-requirements
			randomPassword, err := random.NewRandomString(e.Ctx(), e.Namer.ResourceName(name, "win-admin-password"), &random.RandomStringArgs{
				Length:  pulumi.Int(20),
				Special: pulumi.Bool(true),
				// Disallow "<", ">" and "&" as they get encoded by json.Marshall in the CI log output, making the password hard to read
				OverrideSpecial: pulumi.String("!@#$%*()-_=+[]{}:?"),
				MinLower:        pulumi.Int(1),
				MinUpper:        pulumi.Int(1),
				MinNumeric:      pulumi.Int(1),
			}, utils.MergeOptions(opts, e.WithProviders(config.ProviderRandom))...)
			if err != nil {
				return err
			}
			_, err = c.OS.Runner().Command(
				e.CommonNamer().ResourceName("reset-admin-password"),
				&command.Args{
					Create: pulumi.Sprintf("$Password = ConvertTo-SecureString -String '%s' -AsPlainText -Force; Get-LocalUser -Name 'Administrator' | Set-LocalUser -Password $Password", randomPassword.Result),
				}, utils.MergeOptions(opts, e.WithProviders(config.ProviderRandom))...)
			if err != nil {
				return err
			}

			c.Password = randomPassword.Result
		}

		return nil
	})
}

func InstallECRCredentialsHelper(e aws.Environment, vm *remote.Host) (command.Command, error) {
	ecrCredsHelperInstall, err := vm.OS.PackageManager().Ensure("amazon-ecr-credential-helper", nil, "docker-credential-ecr-login")
	if err != nil {
		return nil, err
	}

	ecrConfigCommand, err := vm.OS.Runner().Command(
		e.CommonNamer().ResourceName("ecr-config"),
		&command.Args{
			Create: pulumi.Sprintf("mkdir -p ~/.docker && echo '{\"credsStore\": \"ecr-login\"}' > ~/.docker/config.json"),
			Sudo:   false,
		}, utils.PulumiDependsOn(ecrCredsHelperInstall))
	if err != nil {
		return nil, err
	}

	return ecrConfigCommand, nil
}

func defaultVMArgs(e aws.Environment, vmArgs *vmArgs) error {
	if vmArgs.osInfo == nil {
		vmArgs.osInfo = &os.UbuntuDefault
	}

	if vmArgs.instanceProfile == "" {
		vmArgs.instanceProfile = e.DefaultInstanceProfileName()
	}

	if vmArgs.instanceType == "" {
		vmArgs.instanceType = e.DefaultInstanceType()
		if vmArgs.osInfo.Architecture == os.ARM64Arch {
			vmArgs.instanceType = e.DefaultARMInstanceType()
		}
		if vmArgs.osInfo.Family() == os.WindowsFamily {
			vmArgs.instanceType = e.DefaultWindowsInstanceType()
		}
	}

	if vmArgs.volumeThroughput == 0 && vmArgs.osInfo.Family() == os.WindowsFamily {
		// Increase throughput for Windows instances to 400 MiB/s to reduce test flakiness
		// May be able to lower this if we can disable some on-boot services in custom AMIs
		vmArgs.volumeThroughput = 400
	}

	// macOS dedicated host defaults
	if vmArgs.osInfo.Family() == os.MacOSFamily {
		// default to mac2.metal for arm64 and mac1.metal for amd64 if not set explicitly
		if vmArgs.instanceType == "" || strings.HasPrefix(vmArgs.instanceType, "t3.") || strings.HasPrefix(vmArgs.instanceType, "t4g.") {
			if vmArgs.osInfo.Architecture == os.ARM64Arch {
				vmArgs.instanceType = "mac2.metal"
			} else {
				vmArgs.instanceType = "mac1.metal"
			}
		}
		if vmArgs.tenancy == "" {
			vmArgs.tenancy = "host"
		}
	}

	// Handle custom user data and defaults per os
	defaultUserData := ""
	if vmArgs.osInfo.Family() == os.WindowsFamily {
		var err error
		defaultUserData, err = getWindowsOpenSSHUserData(e.DefaultPublicKeyPath())
		if err != nil {
			return err
		}
	} else if vmArgs.osInfo.Flavor == os.Ubuntu || vmArgs.osInfo.Flavor == os.Debian {
		defaultUserData = os.APTDisableUnattendedUpgradesScriptContent
	} else if vmArgs.osInfo.Flavor == os.Suse {
		defaultUserData = os.ZypperDisableUnattendedUpgradesScriptContent
	}
	userDataParts := make([]string, 0, 3)
	if vmArgs.userData != "" {
		userDataParts = append(userDataParts, vmArgs.userData)
	}
	if defaultUserData != "" {
		userDataParts = append(userDataParts, defaultUserData)
	}
	if vmArgs.osInfo.Family() == os.LinuxFamily {
		userDataParts = append(userDataParts, os.SSHAllowSFTPRootScriptContent)
	}
	vmArgs.userData = strings.Join(userDataParts, "\n")

	return nil
}
