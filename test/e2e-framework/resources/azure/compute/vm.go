// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package compute

import (
	_ "embed"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"

	componentsos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"

	compute "github.com/pulumi/pulumi-azure-native-sdk/compute/v2"
	network "github.com/pulumi/pulumi-azure-native-sdk/network/v2"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	imageURNSeparator = ":"
	AdminUsername     = "azureuser"
)

func NewLinuxInstance(e azure.Environment, name, imageUrn, instanceType string, userData pulumi.StringPtrInput, opts ...pulumi.ResourceOption) (vm *compute.VirtualMachine, privateIP pulumi.StringOutput, err error) {
	sshPublicKey, err := utils.GetSSHPublicKey(e.DefaultPublicKeyPath())
	if err != nil {
		return nil, pulumi.StringOutput{}, err
	}

	linuxOsProfile := compute.OSProfileArgs{
		ComputerName:  pulumi.String(name),
		AdminUsername: pulumi.String(AdminUsername),
		LinuxConfiguration: compute.LinuxConfigurationArgs{
			DisablePasswordAuthentication: pulumi.BoolPtr(true),
			Ssh: compute.SshConfigurationArgs{
				PublicKeys: compute.SshPublicKeyTypeArray{
					compute.SshPublicKeyTypeArgs{
						KeyData: sshPublicKey,
						Path:    pulumi.StringPtr(fmt.Sprintf("/home/%s/.ssh/authorized_keys", AdminUsername)),
					},
				},
			},
		},
		CustomData: userData,
	}

	vm, networkInterface, err := newVMInstance(e, name, imageUrn, instanceType, linuxOsProfile, opts...)
	if err != nil {
		return nil, pulumi.StringOutput{}, err
	}
	return vm, networkInterface.IpConfigurations.Index(pulumi.Int(0)).PrivateIPAddress().Elem(), nil
}

//go:embed setup-ssh-param.ps1
var setupSSHParamScriptContent string

func NewWindowsInstance(e azure.Environment, name, imageUrn, instanceType string, userData, firstLogonCommand pulumi.StringPtrInput, opts ...pulumi.ResourceOption) (vm *compute.VirtualMachine, privateIP pulumi.StringOutput, password pulumi.StringOutput, err error) {
	pwdOpts := make([]pulumi.ResourceOption, 0, len(opts)+1)
	copy(pwdOpts, opts)
	pwdOpts = append(pwdOpts, e.WithProviders(config.ProviderRandom))
	windowsAdminPassword, err := random.NewRandomString(e.Ctx(), e.Namer.ResourceName(name, "admin-password"), &random.RandomStringArgs{
		Length:  pulumi.Int(20),
		Special: pulumi.Bool(true),
		// Disallow "<", ">" and "&" as they get encoded by json.Marshall in the CI log output, making the password hard to read
		OverrideSpecial: pulumi.String("!@#$%*()-_=+[]{}:?"),
	}, pwdOpts...)
	if err != nil {
		return nil, pulumi.StringOutput{}, pulumi.StringOutput{}, err
	}

	windowsOsProfile := compute.OSProfileArgs{
		ComputerName:  pulumi.String(name),
		AdminUsername: pulumi.String(AdminUsername),
		AdminPassword: windowsAdminPassword.Result,
		CustomData:    userData,
	}

	if firstLogonCommand != nil {
		windowsOsProfile.WindowsConfiguration = compute.WindowsConfigurationArgs{
			AdditionalUnattendContent: compute.AdditionalUnattendContentArray{
				compute.AdditionalUnattendContentArgs{
					ComponentName: compute.ComponentNames_Microsoft_Windows_Shell_Setup,
					PassName:      compute.PassNamesOobeSystem,
					SettingName:   compute.SettingNamesFirstLogonCommands,
					Content:       firstLogonCommand,
				},
			},
		}
	}

	vm, nw, err := newVMInstance(e, name, imageUrn, instanceType, windowsOsProfile, opts...)
	if err != nil {
		return nil, pulumi.StringOutput{}, pulumi.StringOutput{}, err
	}

	publicKey, err := os.ReadFile(e.DefaultPublicKeyPath())
	if err != nil {
		return nil, pulumi.StringOutput{}, pulumi.StringOutput{}, err
	}

	setupSSHCommand, err := compute.NewVirtualMachineRunCommandByVirtualMachine(e.Ctx(), fmt.Sprintf("%s-init-cmd", name), &compute.VirtualMachineRunCommandByVirtualMachineArgs{
		ResourceGroupName: pulumi.String(e.DefaultResourceGroup()),
		VmName:            vm.Name,
		AsyncExecution:    pulumi.Bool(false),
		RunCommandName:    pulumi.String("InitVM"),
		Source: compute.VirtualMachineRunCommandScriptSourceArgs{
			Script: pulumi.String(strings.Join([]string{setupSSHParamScriptContent, componentsos.WindowsSetupSSHScriptContent}, "\n\n")),
		},
		Parameters: compute.RunCommandInputParameterArray{
			compute.RunCommandInputParameterArgs{
				Name:  pulumi.String("authorizedKey"),
				Value: pulumi.String(publicKey),
			},
		},
		TimeoutInSeconds:                pulumi.Int(120),
		TreatFailureAsDeploymentFailure: pulumi.Bool(true),
	}, pulumi.Parent(vm))

	if err != nil {
		return nil, pulumi.StringOutput{}, pulumi.StringOutput{}, err
	}

	privateIP = pulumi.All(nw.IpConfigurations.Index(pulumi.Int(0)).PrivateIPAddress().Elem(), setupSSHCommand.URN()).ApplyT(func(args []interface{}) string {
		return args[0].(string)
	}).(pulumi.StringOutput)

	return vm, privateIP, windowsAdminPassword.Result, nil
}

func newVMInstance(e azure.Environment, name, imageUrn, instanceType string, osProfile compute.OSProfilePtrInput, opts ...pulumi.ResourceOption) (*compute.VirtualMachine, *network.NetworkInterface, error) {
	vmImageRef, err := parseImageReferenceURN(imageUrn)
	if err != nil {
		return nil, nil, err
	}

	nwOpts := make([]pulumi.ResourceOption, 0, len(opts)+1)
	copy(nwOpts, opts)
	nwOpts = append(nwOpts, e.WithProviders(config.ProviderAzure))
	nwInt, err := network.NewNetworkInterface(e.Ctx(), e.Namer.ResourceName(name), &network.NetworkInterfaceArgs{
		NetworkInterfaceName: e.Namer.DisplayName(math.MaxInt, pulumi.String(name)),
		ResourceGroupName:    pulumi.String(e.DefaultResourceGroup()),
		NetworkSecurityGroup: network.NetworkSecurityGroupTypeArgs{
			Id: pulumi.String(e.DefaultSecurityGroup()),
		},
		IpConfigurations: network.NetworkInterfaceIPConfigurationArray{
			network.NetworkInterfaceIPConfigurationArgs{
				Name: e.Namer.DisplayName(math.MaxInt, pulumi.String(name)),
				Subnet: network.SubnetTypeArgs{
					Id: pulumi.String(e.DefaultSubnet()),
				},
				PrivateIPAllocationMethod: pulumi.String(network.IPAllocationMethodDynamic),
			},
		},
		Tags: e.ResourcesTags(),
	}, nwOpts...)
	if err != nil {
		return nil, nil, err
	}

	vmOpts := make([]pulumi.ResourceOption, 0, len(opts)+1)
	copy(vmOpts, opts)
	vmOpts = append(vmOpts, e.WithProviders(config.ProviderAzure))
	vm, err := compute.NewVirtualMachine(e.Ctx(), e.Namer.ResourceName(name), &compute.VirtualMachineArgs{
		ResourceGroupName: pulumi.String(e.DefaultResourceGroup()),
		VmName:            e.Namer.DisplayName(math.MaxInt, pulumi.String(name)),
		HardwareProfile: compute.HardwareProfileArgs{
			VmSize: pulumi.StringPtr(instanceType),
		},
		StorageProfile: compute.StorageProfileArgs{
			OsDisk: compute.OSDiskArgs{
				Name:         e.Namer.DisplayName(math.MaxInt, pulumi.String(name), pulumi.String("os-disk")),
				CreateOption: pulumi.String(compute.DiskCreateOptionFromImage),
				ManagedDisk: compute.ManagedDiskParametersArgs{
					StorageAccountType: pulumi.String("StandardSSD_LRS"),
				},
				DeleteOption: pulumi.String(compute.DiskDeleteOptionTypesDelete),
				DiskSizeGB:   pulumi.IntPtr(200), // Windows requires at least 127GB
			},
			ImageReference: vmImageRef,
		},
		NetworkProfile: compute.NetworkProfileArgs{
			NetworkInterfaces: compute.NetworkInterfaceReferenceArray{
				compute.NetworkInterfaceReferenceArgs{
					Id: nwInt.ID(),
				},
			},
		},
		OsProfile: osProfile,
		Tags:      e.ResourcesTags(),
	}, vmOpts...)
	if err != nil {
		return nil, nil, err
	}

	return vm, nwInt, nil
}
