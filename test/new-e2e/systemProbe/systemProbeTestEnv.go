// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package systemProbe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/credentials"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
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

var SSHKeyFile = filepath.Join("/", "tmp", "aws-ssh-key")
var VMConfig = filepath.Join(".", "systemProbe", "config", "vmconfig.json")

func NewTestEnv(name string) (*TestEnv, error) {
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
		"ddinfra:aws/defaultARMInstanceType": auto.ConfigValue{Value: "m6g.metal"},
		"ddinfra:aws/defaultInstanceType":    auto.ConfigValue{Value: "z1d.metal"},
		"microvm:microVMConfigFile":          auto.ConfigValue{Value: VMConfig},
		"ddinfra:aws/defaultKeyPairName":     auto.ConfigValue{Value: "aws-ssh-key"},
		"ddinfra:aws/defaultPrivateKeyPath":  auto.ConfigValue{Value: SSHKeyFile},
	}

	upResult, err := stackManager.GetStack(systemProbeTestEnv.context, systemProbeTestEnv.envName, systemProbeTestEnv.name, config, func(ctx *pulumi.Context) error {
		err := microVMs.Run(ctx)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return systemProbeTestEnv, nil
}

func (testEnv *TestEnv) Destroy() error {
	return infra.GetStackManager().DeleteStack(testEnv.context, testEnv.envName, testEnv.name)
}
