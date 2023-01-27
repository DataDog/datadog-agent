// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package systemProbe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/credentials"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
	"github.com/DataDog/test-infra-definitions/aws"
	"github.com/DataDog/test-infra-definitions/aws/scenarios/microVMs/microVMs"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type TestEnv struct {
	context context.Context
	envName string
	name    string

	Arm64InstanceIP  string
	X86_64InstanceIP string
	StackOutput      auto.UpResult
}

// go:embed config/vmconfig.json
//var vmconfig string

const (
	composeDataPath = "compose/data"
)

var (
	SSHKeyFile           = filepath.Join("/", "tmp", "aws-ssh-key")
	VMConfig             = filepath.Join(".", "systemProbe", "config", "vmconfig.json")
	DD_AGENT_TESTING_DIR = os.Getenv("DD_AGENT_TESTING_DIR")
)

func NewTestEnv(name, securityGroups, subnets, x86InstanceType, armInstanceType string) (*TestEnv, error) {
	systemProbeTestEnv := &TestEnv{
		context: context.Background(),
		envName: "aws/sandbox",
		name:    fmt.Sprintf("microvm-scenario-%s", name),
	}

	awsManager := credentials.NewManager()
	sshkey, err := awsManager.GetCredential(credentials.AWSSSMStore, "ci.datadog-agent.aws_ec2_kitchen_ssh_key")
	if err != nil {
		return nil, err
	}

	// Write ssh key to file
	f, err := os.Create(SSHKeyFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	f.WriteString(sshkey)

	stackManager := infra.GetStackManager()

	config := auto.ConfigMap{
		"ddinfra:aws/defaultARMInstanceType": auto.ConfigValue{Value: armInstanceType},
		"ddinfra:aws/defaultInstanceType":    auto.ConfigValue{Value: x86InstanceType},
		"microvm:microVMConfigFile":          auto.ConfigValue{Value: VMConfig},
		"ddinfra:aws/defaultKeyPairName":     auto.ConfigValue{Value: "datadog-agent-kitchen"},
		"ddinfra:aws/defaultPrivateKeyPath":  auto.ConfigValue{Value: SSHKeyFile},
		"ddinfra:aws/defaultSecurityGroups":  auto.ConfigValue{Value: securityGroups},
		"ddinfra:aws/defaultSubnets":         auto.ConfigValue{Value: subnets},
	}

	upResult, err := stackManager.GetStack(systemProbeTestEnv.context, systemProbeTestEnv.envName, systemProbeTestEnv.name, config, func(ctx *pulumi.Context) error {
		awsEnvironment, err := aws.AWSEnvironment(ctx)
		if err != nil {
			return err
		}

		_, err = microVMs.RunAndReturnInstances(ctx, awsEnvironment)
		if err != nil {
			return err
		}

		//for _, instance := range scenarioDone.Instances {
		//	localRunner
		//	//remoteRunner, err := command.NewRunner(*awsEnvironment.CommonEnvironment, "remote-runner-"+instance.Arch, instance.Connection, func(r *command.Runner) (*remote.Command, error) {
		//	//	return command.WaitForCloudInit(awsEnvironment.Ctx, r)
		//	//})

		//	//filemanager := command.NewFileManager(remoteRunner)
		//	//_, err = filemanager.CopyFile(
		//	//	fmt.Sprintf("%s/site-cookbooks-%s.tar.gz", DD_AGENT_TESTING_DIR, instance.Arch),
		//	//	"/tmp",
		//	//	pulumi.DependsOn(scenarioDone.Dependencies),
		//	//)
		//	//if err != nil {
		//	//	return err
		//	//}
		//}

		return nil
	})

	if err != nil {
		return nil, err
	}

	systemProbeTestEnv.StackOutput = upResult

	f2, err := os.Create("/tmp/test123.txt")
	if err != nil {
		return nil, err
	}
	f2.WriteString("testing\n")
	f2.Close()

	outputX86, found := upResult.Outputs["x86_64-instance-ip"]
	if found {
		systemProbeTestEnv.X86_64InstanceIP = outputX86.Value.(string)

		cmd1 := exec.Command(fmt.Sprintf("ls -lh %s", SSHKeyFile))
		err := cmd1.Run()
		if err != nil {
			return nil, err
		}

		cmd2 := exec.Command("ls -lh /tmp/test123.txt")
		err = cmd2.Run()
		if err != nil {
			return nil, err
		}

		cmd := exec.Command(fmt.Sprintf("scp -i %s /tmp/test123.txt %s:/tmp", SSHKeyFile, systemProbeTestEnv.X86_64InstanceIP))
		err = cmd.Run()
		if err != nil {
			return nil, err
		}
	}

	return systemProbeTestEnv, nil
}

func (testEnv *TestEnv) Destroy() error {
	return infra.GetStackManager().DeleteStack(testEnv.context, testEnv.envName, testEnv.name)
}
