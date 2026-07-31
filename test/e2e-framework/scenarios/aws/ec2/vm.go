// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"fmt"
	stdos "os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2/pool"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"

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

		// isMacOSPoolMember/poolAcquired drive the pool-acquire/pool-provision/
		// pool-release wiring below, once the instance itself has been created or
		// imported.
		isMacOSPoolMember := vmArgs.osInfo.Family() == os.MacOSFamily && vmArgs.hostID == ""
		isCI, _ := strconv.ParseBool(stdos.Getenv("CI"))
		username := e.Username()
		stackID := e.Ctx().Stack()
		var poolAcquired pool.AcquireResult

		if isMacOSPoolMember {
			poolClient, err := pool.NewEC2Client(e.Ctx().Context(), e.Region(), e.Profile())
			if err != nil {
				return err
			}

			// A local run is scoped to the developer's own previously-provisioned
			// instance via the username tag. Found == false means they own none yet,
			// so provision one; an existing but busy instance is an error instead.
			var localOpts *pool.LocalProvisionOptions
			if !isCI {
				localOpts = &pool.LocalProvisionOptions{Username: username}
			}

			poolAcquired, err = pool.Acquire(e.Ctx().Context(), e.Region(), e.Profile(), poolClient, stackID, localOpts)
			if err != nil {
				return err
			}

			if localOpts != nil && poolAcquired.Found {
				revertBeforeRun, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.RevertBeforeRun, false)
				if err != nil {
					return err
				}
				if revertBeforeRun {
					// The re-published lease has a new ETag; keep it or the release at
					// teardown fails its If-Match and strands the lease as in-use.
					newToken, err := pool.RevertInPlace(e.Ctx().Context(), e.Region(), e.Profile(), poolAcquired.InstanceID, poolAcquired.LeaseToken)
					if err != nil {
						return fmt.Errorf("failed to revert local pool instance %s before run: %w", poolAcquired.InstanceID, err)
					}
					poolAcquired.LeaseToken = newToken
				}
			}

			// Pooled instances/hosts are never destroyed by Pulumi;
			// BaseSuite.releasePoolInstanceIfAny releases them back to idle instead.
			opts = append(opts, pulumi.RetainOnDelete(true))
			instanceArgs.Tenancy = "host"

			if poolAcquired.Found {
				// Import the existing pool member instead of creating a new instance,
				// and pin HostID/SubnetID to what it's actually running on, since the
				// instance's AZ is fixed by its Dedicated Host.
				opts = append(opts, pulumi.Import(pulumi.ID(poolAcquired.InstanceID)))
				instanceArgs.HostID = pulumi.String(poolAcquired.HostID)
				instanceArgs.SubnetID = pulumi.String(poolAcquired.SubnetID)

				// Tags, AMI, and key pair are owned externally on an imported pool
				// member; without IgnoreChanges, Pulumi would reconcile them down to
				// NewInstance's defaults, stripping the pool tag and drifting the AMI.
				opts = append(opts, pulumi.IgnoreChanges([]string{"tags", "ami", "keyName"}))

				// Exported so BaseSuite.releasePoolInstanceIfAny can revert and release
				// the instance once the test suite completes, independent of region/
				// profile resolution happening again on the test-harness side.
				c.PoolInstanceID = pulumi.String(poolAcquired.InstanceID).ToStringOutput()
				c.PoolLeaseToken = pulumi.String(poolAcquired.LeaseToken).ToStringOutput()
				// Already registered, so no baseline to hand to the harness.
				c.PoolBaselineImageID = pulumi.String("").ToStringOutput()
				c.PoolStackID = pulumi.String(stackID).ToStringOutput()
			} else {
				// Local cache miss: no pool member owned by this developer exists yet.
				// Provision a new Dedicated Host + instance through ordinary Pulumi
				// resources so they're tracked in the stack.
				host, err := ec2.NewDedicatedHost(e, name, ec2.DedicatedHostArgs{InstanceType: vmArgs.instanceType}, opts...)
				if err != nil {
					return err
				}
				instanceArgs.HostID = host.ID()
			}
		} else {
			c.PoolInstanceID = pulumi.String("").ToStringOutput()
			c.PoolLeaseToken = pulumi.String("").ToStringOutput()
			c.PoolBaselineImageID = pulumi.String("").ToStringOutput()
			c.PoolStackID = pulumi.String("").ToStringOutput()
		}

		if isMacOSPoolMember {
			c.PoolRegion = pulumi.String(e.Region()).ToStringOutput()
			c.PoolProfile = pulumi.String(e.Profile()).ToStringOutput()
		} else {
			c.PoolRegion = pulumi.String("").ToStringOutput()
			c.PoolProfile = pulumi.String("").ToStringOutput()
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
			remote.WithDialErrorLimit(e.InfraDialErrorLimit()),
			remote.WithPerDialTimeoutSeconds(e.InfraPerDialTimeoutSeconds()),
		)
		if err != nil {
			return err
		}

		err = remote.InitHost(&e, conn.ToConnectionOutput(), *vmArgs.osInfo, sshUser, pulumi.String("").ToStringOutput(), amiInfo.readyFunc, c)

		if err != nil {
			return err
		}

		if isMacOSPoolMember && !poolAcquired.Found {
			// Freshly created local instance: bake its current disk state into a golden
			// AMI now that InitHost's setup has completed. The runner's options carry a
			// DependsOn its readiness command, which is what orders the AMI after setup.
			// PulumiOptions already carries Parent(c) and DeletedWith(c) as well.
			imageID, err := ec2.RegisterPoolMember(e, name, instance.ID().ToStringOutput(), username,
				c.OS.Runner().PulumiOptions()...)
			if err != nil {
				return err
			}
			c.PoolInstanceID = instance.ID().ToStringOutput()
			c.PoolBaselineImageID = imageID
			c.PoolStackID = pulumi.String(stackID).ToStringOutput()
			// Left empty on purpose: an empty token with a set instance ID is how
			// BaseSuite recognises a member whose first lease still needs publishing.
			c.PoolLeaseToken = pulumi.String("").ToStringOutput()
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
