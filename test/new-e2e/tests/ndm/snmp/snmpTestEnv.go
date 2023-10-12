// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package snmp contains e2e tests for snmp
package snmp

import (
	"context"
	"embed"
	"errors"
	"path"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent/docker"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent/dockerparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// TestEnv implements a test environment for NDM. Deprecated, should port to TestSuite
type TestEnv struct {
	context context.Context
	name    string

	InstanceIP  string
	StackOutput auto.UpResult
}

//go:embed compose/snmpCompose.yaml
var snmpCompose string

//go:embed config/public.yaml
var snmpConfig string

const (
	composeDataPath = "compose/data"
)

// NewTestEnv creates a new test environment for NDM. Deprecated, should port to NDM
func NewTestEnv() (*TestEnv, error) {
	snmpTestEnv := &TestEnv{
		context: context.Background(),
		name:    "snmp-agent",
	}

	stackManager := infra.GetStackManager()

	_, upResult, err := stackManager.GetStack(snmpTestEnv.context, snmpTestEnv.name, nil, func(ctx *pulumi.Context) error {
		// setup VM
		vm, err := ec2vm.NewUnixEc2VM(ctx)
		if err != nil {
			return err
		}

		filemanager := vm.GetFileManager()
		// upload snmpsim data files
		createDataDirCommand, dataPath, err := filemanager.TempDirectory("data")
		if err != nil {
			return err
		}
		dataFiles, err := loadDataFileNames()
		if err != nil {
			return err
		}
		fileCommands := []pulumi.Resource{}
		for _, fileName := range dataFiles {
			fileContent, err := dataFolder.ReadFile(path.Join(composeDataPath, fileName))
			if err != nil {
				return err
			}
			dontUseSudo := false
			fileCommand, err := filemanager.CopyInlineFile(pulumi.String(fileContent), path.Join(dataPath, fileName), dontUseSudo,
				pulumi.DependsOn([]pulumi.Resource{createDataDirCommand}))
			if err != nil {
				return err
			}
			fileCommands = append(fileCommands, fileCommand)
		}

		createConfigDirCommand, configPath, err := filemanager.TempDirectory("config")
		if err != nil {
			return err
		}
		// edit snmp config file
		dontUseSudo := false
		configCommand, err := filemanager.CopyInlineFile(pulumi.String(snmpConfig), path.Join(configPath, "snmp.yaml"), dontUseSudo,
			pulumi.DependsOn([]pulumi.Resource{createConfigDirCommand}))
		if err != nil {
			return err
		}

		// install agent and snmpsim on docker
		envVars := map[string]string{"DATA_DIR": dataPath, "CONFIG_DIR": configPath}
		composeDependencies := []pulumi.Resource{createDataDirCommand, configCommand}
		composeDependencies = append(composeDependencies, fileCommands...)
		_, err = docker.NewDaemon(
			ctx,
			dockerparams.WithComposeContent(snmpCompose, envVars),
			dockerparams.WithPulumiResources(pulumi.DependsOn(composeDependencies)),
		)
		return err
	}, false)
	if err != nil {
		return nil, err
	}

	snmpTestEnv.StackOutput = upResult

	output, found := upResult.Outputs["instance-ip"]

	if !found {
		return nil, errors.New("unable to find host ip")
	}
	snmpTestEnv.InstanceIP = output.Value.(string)

	return snmpTestEnv, nil
}

// Destroy delete the NDM stack. Deprecated, should port to NDM
func (testEnv *TestEnv) Destroy() error {
	return infra.GetStackManager().DeleteStack(testEnv.context, testEnv.name)
}

//go:embed compose/data
var dataFolder embed.FS

func loadDataFileNames() (out []string, err error) {
	fileEntries, err := dataFolder.ReadDir(composeDataPath)
	if err != nil {
		return nil, err
	}
	for _, f := range fileEntries {
		out = append(out, f.Name())
	}
	return out, nil
}
