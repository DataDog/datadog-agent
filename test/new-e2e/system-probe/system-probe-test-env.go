// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package systemProbe

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	commonConfig "github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/microVMs/microvms"
	pulumiCommand "github.com/pulumi/pulumi-command/sdk/go/command"
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/sethvargo/go-retry"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

var (
	DependenciesPackage = "dependencies-%s.tar.gz"
)

type SystemProbeEnvOpts struct {
	X86AmiID              string
	ArmAmiID              string
	SSHKeyPath            string
	SSHKeyName            string
	InfraEnv              string
	Provision             bool
	ShutdownPeriod        int
	FailOnMissing         bool
	UploadDependencies    bool
	DependenciesDirectory string
	Subnets               string
}

type TestEnv struct {
	context context.Context
	name    string

	ARM64InstanceIP  string
	X86_64InstanceIP string
	StackOutput      auto.UpResult
}

var (
	MicroVMsDependenciesPath = filepath.Join("/", "opt", "kernel-version-testing", "dependencies-%s.tar.gz")
	CustomAMIWorkingDir      = filepath.Join("/", "home", "kernel-version-testing")
	vmConfig                 = filepath.Join(".", "system-probe", "config", "vmconfig.json")

	CI_PROJECT_DIR = GetEnv("CI_PROJECT_DIR", "/tmp")
	sshKeyX86      = GetEnv("LibvirtSSHKeyX86", "/tmp/libvirt_rsa-x86_64")
	sshKeyArm      = GetEnv("LibvirtSSHKeyARM", "/tmp/libvirt_rsa-arm64")

	stackOutputs = filepath.Join(CI_PROJECT_DIR, "stack.outputs")
)

func outputsToFile(output auto.OutputMap) error {
	f, err := os.Create(stackOutputs)
	if err != nil {
		return fmt.Errorf("failed to create file: %s: %w", stackOutputs, err)
	}
	defer f.Close()

	for key, value := range output {
		if _, err := f.WriteString(fmt.Sprintf("%s %s\n", key, value.Value.(string))); err != nil {
			return fmt.Errorf("write string: %s", err)
		}
	}
	return f.Sync()
}

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func NewTestEnv(name, x86InstanceType, armInstanceType string, opts *SystemProbeEnvOpts) (*TestEnv, error) {
	var err error
	systemProbeTestEnv := &TestEnv{
		context: context.Background(),
		name:    fmt.Sprintf("microvm-scenario-%s", name),
	}

	stackManager := infra.GetStackManager()

	config := runner.ConfigMap{
		runner.InfraEnvironmentVariables: auto.ConfigValue{Value: opts.InfraEnv},
		runner.AWSKeyPairName:            auto.ConfigValue{Value: opts.SSHKeyName},
		// Its fine to hardcode the password here, since the remote ec2 instances do not have
		// any password on sudo. This secret configuration was introduced in the test-infra-definitions
		// scenario for dev environments: https://github.com/DataDog/test-infra-definitions/pull/159
		"sudo-password-remote":                   auto.ConfigValue{Value: "", Secret: true},
		"ddinfra:aws/defaultARMInstanceType":     auto.ConfigValue{Value: armInstanceType},
		"ddinfra:aws/defaultInstanceType":        auto.ConfigValue{Value: x86InstanceType},
		"ddinfra:aws/defaultShutdownBehavior":    auto.ConfigValue{Value: "terminate"},
		"ddinfra:aws/defaultInstanceStorageSize": auto.ConfigValue{Value: "500"},
		"microvm:microVMConfigFile":              auto.ConfigValue{Value: vmConfig},
		"microvm:libvirtSSHKeyFileX86":           auto.ConfigValue{Value: sshKeyX86},
		"microvm:libvirtSSHKeyFileArm":           auto.ConfigValue{Value: sshKeyArm},
		"microvm:provision":                      auto.ConfigValue{Value: "false"},
		"microvm:x86AmiID":                       auto.ConfigValue{Value: opts.X86AmiID},
		"microvm:arm64AmiID":                     auto.ConfigValue{Value: opts.ArmAmiID},
		"microvm:workingDir":                     auto.ConfigValue{Value: CustomAMIWorkingDir},
		"microvm:shutdownPeriod":                 auto.ConfigValue{Value: strconv.Itoa(opts.ShutdownPeriod)},
	}
	// We cannot add defaultPrivateKeyPath if the key is in ssh-agent, otherwise passphrase is needed
	if opts.SSHKeyPath != "" {
		config["ddinfra:aws/defaultPrivateKeyPath"] = auto.ConfigValue{Value: opts.SSHKeyPath}
	} else {
		config["ddinfra:aws/defaultPrivateKeyPath"] = auto.ConfigValue{Value: ""}
	}

	// Specify the subnets to use instead of default ones
	if opts.Subnets != "" {
		config["ddinfra:aws/defaultSubnets"] = auto.ConfigValue{Value: opts.Subnets}
	}

	var upResult auto.UpResult
	ctx := context.Background()
	b := retry.NewConstant(3 * time.Second)
	b = retry.WithMaxRetries(3, b)
	if retryErr := retry.Do(ctx, b, func(_ context.Context) error {
		_, upResult, err = stackManager.GetStack(systemProbeTestEnv.context, systemProbeTestEnv.name, config, func(ctx *pulumi.Context) error {
			commonEnv, err := commonConfig.NewCommonEnvironment(ctx)
			if err != nil {
				return fmt.Errorf("common environment: %w", err)
			}

			scenarioDone, err := microvms.RunAndReturnInstances(commonEnv)
			if err != nil {
				return fmt.Errorf("setup micro-vms in remote instance: %w", err)
			}

			osCommand := command.NewUnixOSCommand()
			commandProvider, err := pulumiCommand.NewProvider(ctx, "test-env-command-provider", &pulumiCommand.ProviderArgs{})
			if err != nil {
				return fmt.Errorf("failed to get command provider: %w", err)
			}
			for _, instance := range scenarioDone.Instances {
				remoteRunner, err := command.NewRunner(commonEnv, command.RunnerArgs{
					ConnectionName: "remote-runner-" + instance.Arch,
					Connection:     instance.Connection,
					ReadyFunc: func(r *command.Runner) (*remote.Command, error) {
						return command.WaitForCloudInit(r)
					},
					OSCommand: osCommand,
				})
				if err != nil {
					return fmt.Errorf("new runner: %w", err)
				}

				if opts.UploadDependencies {
					// Copy dependencies to micro-vms. Directory '/opt/kernel-version-testing'
					// is mounted to all micro-vms. Each micro-vm extract the context on boot.
					filemanager := command.NewFileManager(remoteRunner)
					_, err = filemanager.CopyFile(
						filepath.Join(opts.DependenciesDirectory, fmt.Sprintf(DependenciesPackage, instance.Arch)),
						fmt.Sprintf(MicroVMsDependenciesPath, instance.Arch),
						pulumi.DependsOn(scenarioDone.Dependencies),
						pulumi.Provider(commandProvider),
					)
					if err != nil {
						return fmt.Errorf("copy file: %w", err)
					}
				}
			}

			return nil
		}, opts.FailOnMissing)
		// Only retry if we failed to dial libvirt.
		// Libvirt daemon on the server occasionally crashes with the following error
		// "End of file while reading data: Input/output error"
		// The root cause of this is unknown. The problem usually fixes itself upon retry.
		if err != nil {
			if strings.Contains(err.Error(), "failed to dial libvirt") {
				fmt.Printf("[Error] Failed to dial libvirt. Retrying stack.")
				return retry.RetryableError(err)
			} else {
				return err
			}
		}

		return nil
	}); retryErr != nil {
		return nil, fmt.Errorf("failed to create stack: %w", retryErr)
	}

	err = outputsToFile(upResult.Outputs)
	if err != nil {
		return nil, fmt.Errorf("failed to write stack output to file: %w", err)
	}

	systemProbeTestEnv.StackOutput = upResult

	return systemProbeTestEnv, nil
}

func Destroy(name string) error {
	return infra.GetStackManager().DeleteStack(context.Background(), name)
}
