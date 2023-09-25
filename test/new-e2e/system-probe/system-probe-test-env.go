// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package systemProbe

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/microVMs/microvms"
	"github.com/sethvargo/go-retry"
	"golang.org/x/term"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	AgentQAPrimaryAZ   = "subnet-03061a1647c63c3c3"
	AgentQASecondaryAZ = "subnet-0f1ca3e929eb3fb8b"
	AgentQABackupAZ    = "subnet-071213aedb0e1ae54"

	SandboxPrimaryAz   = "subnet-b89e00e2"
	SandboxSecondaryAz = "subnet-8ee8b1c6"
	SandboxBackupAz    = "subnet-3f5db45b"

	DatadogAgentQAEnv = "aws/agent-qa"
	SandboxEnv        = "aws/sandbox"

	Aria2cMissingStatusError = "error: wait: remote command exited without exit status or exit signal: running \" aria2c"
)

var availabilityZones = map[string][]string{
	DatadogAgentQAEnv: {AgentQAPrimaryAZ, AgentQASecondaryAZ, AgentQABackupAZ},
	SandboxEnv:        {SandboxPrimaryAz, SandboxSecondaryAz, SandboxBackupAz},
}

type SystemProbeEnvOpts struct {
	X86AmiID              string
	ArmAmiID              string
	SSHKeyPath            string
	SSHKeyName            string
	InfraEnv              string
	Provision             bool
	ShutdownPeriod        int
	FailOnMissing         bool
	DependenciesDirectory string
	VMConfigPath          string
	Local                 bool
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

func credentials() (string, error) {
	var fd int
	if term.IsTerminal(syscall.Stdin) {
		fd = syscall.Stdin
	} else {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return "", fmt.Errorf("error allocating terminal: %w", err)
		}
		defer tty.Close()
		fd = int(tty.Fd())
	}
	fmt.Print("Enter Password: ")
	bytePassword, err := term.ReadPassword(fd)
	if err != nil {
		return "", err
	}

	password := string(bytePassword)
	return password, nil
}

func getAvailabilityZone(env string, azIndx int) string {
	if zones, ok := availabilityZones[env]; ok {
		return zones[azIndx%len(zones)]
	}

	return ""
}

func NewTestEnv(name, x86InstanceType, armInstanceType string, opts *SystemProbeEnvOpts) (*TestEnv, error) {
	var err error
	var sudoPassword string

	systemProbeTestEnv := &TestEnv{
		context: context.Background(),
		name:    name,
	}

	stackManager := infra.GetStackManager()

	if opts.Local {
		sudoPassword, err = credentials()
		if err != nil {
			return nil, fmt.Errorf("Unable to get password: %w", err)
		}
	} else {
		sudoPassword = ""
	}

	config := runner.ConfigMap{
		runner.InfraEnvironmentVariables: auto.ConfigValue{Value: opts.InfraEnv},
		runner.AWSKeyPairName:            auto.ConfigValue{Value: opts.SSHKeyName},
		// Its fine to hardcode the password here, since the remote ec2 instances do not have
		// any password on sudo. This secret configuration was introduced in the test-infra-definitions
		// scenario for dev environments: https://github.com/DataDog/test-infra-definitions/pull/159
		"sudo-password-remote":                   auto.ConfigValue{Value: "", Secret: true},
		"sudo-password-local":                    auto.ConfigValue{Value: sudoPassword, Secret: true},
		"ddinfra:aws/defaultARMInstanceType":     auto.ConfigValue{Value: armInstanceType},
		"ddinfra:aws/defaultInstanceType":        auto.ConfigValue{Value: x86InstanceType},
		"ddinfra:aws/defaultInstanceStorageSize": auto.ConfigValue{Value: "500"},
		"microvm:microVMConfigFile":              auto.ConfigValue{Value: opts.VMConfigPath},
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
	} else {
		config["ddinfra:aws/defaultPrivateKeyPath"] = auto.ConfigValue{Value: ""}
	}

	if opts.ShutdownPeriod != 0 {
		config["microvm:shutdownPeriod"] = auto.ConfigValue{Value: strconv.Itoa(opts.ShutdownPeriod)}
		config["ddinfra:aws/defaultShutdownBehavior"] = auto.ConfigValue{Value: "terminate"}
	}

	var upResult auto.UpResult
	ctx := context.Background()
	currentAZ := 0 // PrimaryAZ
	b := retry.NewConstant(3 * time.Second)
	// Retry 4 times. This allows us to cycle through all AZs, and handle libvirt
	// connection issues in the worst case.
	b = retry.WithMaxRetries(4, b)
	if retryErr := retry.Do(ctx, b, func(_ context.Context) error {
		if az := getAvailabilityZone(opts.InfraEnv, currentAZ); az != "" {
			config["ddinfra:aws/defaultSubnets"] = auto.ConfigValue{Value: az}
		}

		_, upResult, err = stackManager.GetStackNoDeleteOnFailure(systemProbeTestEnv.context, systemProbeTestEnv.name, config, func(ctx *pulumi.Context) error {
			if err := microvms.Run(ctx); err != nil {
				return fmt.Errorf("setup micro-vms in remote instance: %w", err)
			}
			return nil
		}, opts.FailOnMissing)
		if err != nil {
			// Retry if we failed to dial libvirt.
			// Libvirt daemon on the server occasionally crashes with the following error
			// "End of file while reading data: Input/output error"
			// The root cause of this is unknown. The problem usually fixes itself upon retry.
			if strings.Contains(err.Error(), "failed to dial libvirt") {
				fmt.Println("[Error] Failed to dial libvirt. Retrying stack.")
				return retry.RetryableError(err)

				// Retry if we have capacity issues in our current AZ.
				// We switch to a different AZ and attempt to launch the instance again.
			} else if strings.Contains(err.Error(), "InsufficientInstanceCapacity") {
				fmt.Printf("[Error] Insufficient instance capacity in %s. Retrying stack with %s as the AZ.", getAvailabilityZone(opts.InfraEnv, currentAZ), getAvailabilityZone(opts.InfraEnv, currentAZ+1))
				currentAZ += 1
				return retry.RetryableError(err)

				// Retry when ssh thinks aria2c exited without status. This may happen
				// due to network connectivity issues if ssh keepalive mecahnism fails.
			} else if strings.Contains(err.Error(), Aria2cMissingStatusError) {
				fmt.Println("[Error] Missing exit status from Aria2c. Retrying stack")
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

func (env *TestEnv) RemoveStack() error {
	return infra.GetStackManager().ForceRemoveStackConfiguration(env.context, env.name)
}
