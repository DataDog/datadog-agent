// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmp

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"path"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
	ec2vm "github.com/DataDog/test-infra-definitions/aws/scenarios/vm/ec2VM"
	"github.com/DataDog/test-infra-definitions/datadog/agent/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

type TestEnv struct {
	context context.Context
	envName string
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

func NewTestEnv(name, keyPairName, ddAPIKey, ddAPPKey string) (*TestEnv, error) {
	snmpTestEnv := &TestEnv{
		context: context.Background(),
		envName: "aws/sandbox",
		name:    fmt.Sprintf("snmp-agent-%s", name),
	}

	stackManager := infra.GetStackManager()

	config := auto.ConfigMap{
		"ddagent:apiKey":                 auto.ConfigValue{Value: ddAPIKey, Secret: true},
		"ddinfra:aws/defaultKeyPairName": auto.ConfigValue{Value: keyPairName},
	}

	_, upResult, err := stackManager.GetStack(snmpTestEnv.context, snmpTestEnv.envName, snmpTestEnv.name, config, func(ctx *pulumi.Context) error {
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
			fileCommand, err := filemanager.CopyInlineFile(fileName, pulumi.String(fileContent), path.Join(dataPath, fileName), dontUseSudo,
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
		configCommand, err := filemanager.CopyInlineFile("snmp.yaml", pulumi.String(snmpConfig), path.Join(configPath, "snmp.yaml"), dontUseSudo,
			pulumi.DependsOn([]pulumi.Resource{createConfigDirCommand}))
		if err != nil {
			return err
		}

		// install agent and snmpsim on docker
		envVars := map[string]string{"DATA_DIR": dataPath, "CONFIG_DIR": configPath}
		composeDependencies := []pulumi.Resource{createDataDirCommand, configCommand}
		composeDependencies = append(composeDependencies, fileCommands...)
		_, err = docker.NewAgentDockerInstaller(
			vm,
			docker.WithComposeContent(snmpCompose, envVars),
			docker.WithPulumiResources(pulumi.DependsOn(composeDependencies)),
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

func (testEnv *TestEnv) Destroy() error {
	return infra.GetStackManager().DeleteStack(testEnv.context, testEnv.envName, testEnv.name)
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
