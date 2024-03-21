// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package systemprobe sets up the remote testing environment for system-probe using the Kernel Matrix Testing framework
package systemprobe

import (
	"context"
	_ "embed" // embed files used in this scenario
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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
	agentQAPrimaryAZ   = "subnet-03061a1647c63c3c3"
	agentQASecondaryAZ = "subnet-0f1ca3e929eb3fb8b"
	agentQABackupAZ    = "subnet-071213aedb0e1ae54"

	sandboxPrimaryAz   = "subnet-b89e00e2"
	sandboxSecondaryAz = "subnet-8ee8b1c6"
	sandboxBackupAz    = "subnet-3f5db45b"

	datadogAgentQAEnv = "aws/agent-qa"
	sandboxEnv        = "aws/sandbox"
	ec2TagsEnvVar     = "RESOURCE_TAGS"
)

var availabilityZones = map[string][]string{
	datadogAgentQAEnv: {agentQAPrimaryAZ, agentQASecondaryAZ, agentQABackupAZ},
	sandboxEnv:        {sandboxPrimaryAz, sandboxSecondaryAz, sandboxBackupAz},
}

// EnvOpts are the options for the system-probe scenario
type EnvOpts struct {
	X86AmiID              string
	ArmAmiID              string
	SSHKeyPath            string
	SSHKeyName            string
	InfraEnv              string
	ProvisionInstance     bool
	ProvisionMicrovms     bool
	ShutdownPeriod        int
	FailOnMissing         bool
	DependenciesDirectory string
	VMConfigPath          string
	Local                 bool
	RunAgent              bool
	APIKey                string
	AgentVersion          string
}

// TestEnv represents options for a particular test environment
type TestEnv struct {
	context context.Context
	name    string

	ARM64InstanceIP  string
	X86_64InstanceIP string
	StackOutput      auto.UpResult
}

var (
	ciProjectDir = getEnv("CI_PROJECT_DIR", "/tmp")
	sshKeyX86    = getEnv("LibvirtSSHKeyX86", "/tmp/libvirt_rsa-x86_64")
	sshKeyArm    = getEnv("LibvirtSSHKeyARM", "/tmp/libvirt_rsa-arm64")

	stackOutputs    = filepath.Join(ciProjectDir, "stack.output")
	kmtStackJSONKey = "kmt-stack"
)

func outputsToFile(output auto.OutputMap) error {
	f, err := os.Create(stackOutputs)
	if err != nil {
		return fmt.Errorf("failed to create file: %s: %w", stackOutputs, err)
	}
	defer f.Close()

	for key, value := range output {
		// we only want the json output representing KMT's
		// infrastructure saved to the output file.
		if key != kmtStackJSONKey {
			continue
		}
		switch v := value.Value.(type) {
		case string:
			if _, err := f.WriteString(fmt.Sprintf("%s\n", v)); err != nil {
				return fmt.Errorf("failed to write string to file %q: %v", stackOutputs, err)
			}
		default:
		}
	}
	return f.Sync()
}

func getEnv(key, fallback string) string {
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

// NewTestEnv creates a new test environment
func NewTestEnv(name, x86InstanceType, armInstanceType string, opts *EnvOpts) (*TestEnv, error) {
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

	apiKey := getEnv("DD_API_KEY", "")
	if opts.RunAgent && apiKey == "" {
		return nil, fmt.Errorf("No API Key for datadog-agent provided")
	}

	var customAMILocalWorkingDir string

	// Remote AMI working dir is always on Linux
	customAMIRemoteWorkingDir := filepath.Join("/", "home", "kernel-version-testing")

	if runtime.GOOS == "linux" {
		// Linux share the same working dir as the remote (which is always Linux)
		customAMILocalWorkingDir = customAMIRemoteWorkingDir
	} else if runtime.GOOS == "darwin" {
		// macOS does not let us create /home/kernel-version-testing, so we use an alternative
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		customAMILocalWorkingDir = filepath.Join(homeDir, "kernel-version-testing")
	} else {
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
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
		"microvm:provision-instance":             auto.ConfigValue{Value: strconv.FormatBool(opts.ProvisionInstance)},
		"microvm:provision-microvms":             auto.ConfigValue{Value: strconv.FormatBool(opts.ProvisionMicrovms)},
		"microvm:x86AmiID":                       auto.ConfigValue{Value: opts.X86AmiID},
		"microvm:arm64AmiID":                     auto.ConfigValue{Value: opts.ArmAmiID},
		"microvm:localWorkingDir":                auto.ConfigValue{Value: customAMILocalWorkingDir},
		"microvm:remoteWorkingDir":               auto.ConfigValue{Value: customAMIRemoteWorkingDir},
		"ddagent:deploy":                         auto.ConfigValue{Value: strconv.FormatBool(opts.RunAgent)},
		"ddagent:apiKey":                         auto.ConfigValue{Value: apiKey, Secret: true},
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

	// If no agent version is provided the framework will automatically install the latest agent
	if opts.AgentVersion != "" {
		config["ddagent:version"] = auto.ConfigValue{Value: opts.AgentVersion}
	}

	if envVars := getEnv(ec2TagsEnvVar, ""); envVars != "" {
		config["ddinfra:extraResourcesTags"] = auto.ConfigValue{Value: envVars}
	}

	var upResult auto.UpResult
	var pulumiStack *auto.Stack
	ctx := context.Background()
	currentAZ := 0 // PrimaryAZ
	b := retry.NewConstant(3 * time.Second)
	// Retry 4 times. This allows us to cycle through all AZs, and handle libvirt
	// connection issues in the worst case.
	b = retry.WithMaxRetries(4, b)
	retryErr := retry.Do(ctx, b, func(_ context.Context) error {
		if az := getAvailabilityZone(opts.InfraEnv, currentAZ); az != "" {
			config["ddinfra:aws/defaultSubnets"] = auto.ConfigValue{Value: az}
		}

		pulumiStack, upResult, err = stackManager.GetStackNoDeleteOnFailure(systemProbeTestEnv.context, systemProbeTestEnv.name, config, func(ctx *pulumi.Context) error {
			if err := microvms.Run(ctx); err != nil {
				return fmt.Errorf("setup micro-vms in remote instance: %w", err)
			}
			return nil
		}, opts.FailOnMissing, nil)
		if err != nil {
			return handleScenarioFailure(err, func(possibleError handledError) {
				// handle the following errors by trying in a different availability zone
				if possibleError.errorType == insufficientCapacityError ||
					possibleError.errorType == ec2StateChangeTimeoutError {
					currentAZ++
				}
			})
		}

		return nil
	})

	outputs := upResult.Outputs
	if retryErr != nil {
		// pulumi does not populate `UpResult` with the stack output if the
		// update process failed. In this case we must manually fetch the outputs.
		outputs, err = pulumiStack.Outputs(context.Background())
		if err != nil {
			outputs = nil
			log.Printf("failed to get stack outputs: %v", err)
		}
	}
	err = outputsToFile(outputs)
	if err != nil {
		err = fmt.Errorf("failed to write stack output to file: %w", err)
	}
	if retryErr != nil {
		return nil, errors.Join(fmt.Errorf("failed to create stack: %w", retryErr), err)
	}

	systemProbeTestEnv.StackOutput = upResult

	return systemProbeTestEnv, nil
}

// Destroy deletes the stack with the provided name
func Destroy(name string) error {
	return infra.GetStackManager().DeleteStack(context.Background(), name, nil)
}

// RemoveStack removes the stack configuration with the provided name
func (env *TestEnv) RemoveStack() error {
	return infra.GetStackManager().ForceRemoveStackConfiguration(env.context, env.name)
}
