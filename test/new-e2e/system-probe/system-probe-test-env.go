// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package systemProbe

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/microVMs/microvms"
	"github.com/sethvargo/go-retry"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"

	pulumiCommand "github.com/pulumi/pulumi-command/sdk/go/command"
	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type SystemProbeEnvOpts struct {
	X86AmiID           string
	ArmAmiID           string
	SSHKeyPath         string
	SSHKeyName         string
	InfraEnv           string
	Provision          bool
	ShutdownPeriod     time.Duration
	FailOnMissing      bool
	UploadDependencies bool
}

type TestEnv struct {
	context context.Context
	name    string

	ARM64InstanceIP  string
	X86_64InstanceIP string
	StackOutput      auto.UpResult
}

var (
	CustomAMIWorkingDir = filepath.Join("/", "home", "kernel-version-testing")
	vmConfig            = filepath.Join(".", "system-probe", "config", "vmconfig.json")

	DD_AGENT_TESTING_DIR = os.Getenv("DD_AGENT_TESTING_DIR")
	CI_PROJECT_DIR       = os.Getenv("CI_PROJECT_DIR")
	sshKeyX86            = os.Getenv("LibvirtSSHKeyX86")
	sshKeyArm            = os.Getenv("LibvirtSSHKeyARM")

	stackOutputs = filepath.Join(CI_PROJECT_DIR, "stack.outputs")
)

func outputsToFile(output auto.OutputMap) error {
	f, err := os.Create(stackOutputs)
	if err != nil {
		return fmt.Errorf("failed to create file: %s: %w", stackOutputs, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)

	for key, value := range output {
		fmt.Fprintf(w, "%s %s\n", key, value.Value.(string))
	}
	w.Flush()

	return nil
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
		runner.InfraEnvironmentVariables:         auto.ConfigValue{Value: opts.InfraEnv},
		runner.AwsKeyPairName:                    auto.ConfigValue{Value: opts.SSHKeyName},
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
	}
	// We cannot add defaultPrivateKeyPath if the key is in ssh-agent, otherwise passphrase is needed
	if opts.SSHKeyPath != "" {
		config["ddinfra:aws/defaultPrivateKeyPath"] = auto.ConfigValue{Value: opts.SSHKeyPath}
	}

	var upResult auto.UpResult
	ctx := context.Background()
	b := retry.NewConstant(3 * time.Second)
	b = retry.WithMaxRetries(3, b)
	if retryErr := retry.Do(ctx, b, func(_ context.Context) error {
		_, upResult, err = stackManager.GetStack(systemProbeTestEnv.context, systemProbeTestEnv.name, config, func(ctx *pulumi.Context) error {
			awsEnvironment, err := aws.NewEnvironment(ctx)
			if err != nil {
				return fmt.Errorf("aws new environment: %w", err)
			}

			scenarioDone, err := microvms.RunAndReturnInstances(awsEnvironment)
			if err != nil {
				return fmt.Errorf("setup micro-vms in remote instance: %w", err)
			}

			var depends []pulumi.Resource
			osCommand := command.NewUnixOSCommand()

			commandProvider, err := pulumiCommand.NewProvider(ctx, "test-env-command-provider", &pulumiCommand.ProviderArgs{})
			if err != nil {
				return fmt.Errorf("failed to get command provider: %w", err)
			}
			for _, instance := range scenarioDone.Instances {
				remoteRunner, err := command.NewRunner(*awsEnvironment.CommonEnvironment, command.RunnerArgs{
					ConnectionName: "remote-runner-" + instance.Arch,
					Connection:     instance.Connection,
					ReadyFunc: func(r *command.Runner) (*remote.Command, error) {
						return command.WaitForCloudInit(r)
					},
					OSCommand: osCommand,
				})

				// if shutdown period specified then register a cron job
				// to automatically shutdown the ec2 instance after desired
				// interval. The microvm scenario sets the terminateOnShutdown
				// attribute of the ec2 instance to true. Therefore the shutdown would
				// trigger the automatic termination of the ec2 instance.
				if int64(opts.ShutdownPeriod) > 0 {
					shutdownRegisterArgs := command.Args{
						Create: pulumi.Sprintf(
							"shutdown -P +%.0f", opts.ShutdownPeriod.Minutes(),
						),
						Sudo: true,
					}
					shutdownRegisterDone, err := remoteRunner.Command("shutdown-"+instance.Arch, &shutdownRegisterArgs, pulumi.DependsOn(scenarioDone.Dependencies))
					if err != nil {
						return fmt.Errorf("failed to schedule shutdown: %w", err)
					}
					depends = []pulumi.Resource{shutdownRegisterDone}
				} else {
					depends = scenarioDone.Dependencies
				}

				if opts.UploadDependencies {
					// Copy dependencies to micro-vms. Directory '/opt/kernel-version-testing'
					// is mounted to all micro-vms. Each micro-vm extract the context on boot.
					filemanager := command.NewFileManager(remoteRunner)
					_, err = filemanager.CopyFile(
						fmt.Sprintf("%s/dependencies-%s.tar.gz", DD_AGENT_TESTING_DIR, instance.Arch),
						fmt.Sprintf("/opt/kernel-version-testing/dependencies-%s.tar.gz", instance.Arch),
						pulumi.DependsOn(depends),
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
		// Libvirt daemon on the server occassionally crashes with the following error
		// "End of file while reading data: Input/output error"
		// The root cause of this is unknown. The problem usually fixes itself upon retry.
		if err != nil {
			if strings.Contains(fmt.Sprintf("%s", err), "failed to dial libvirt") {
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

func (testEnv *TestEnv) Destroy() error {
	return infra.GetStackManager().DeleteStack(testEnv.context, testEnv.name)
}
