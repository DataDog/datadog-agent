// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package snmp contains e2e tests for snmp
package snmp

import (
	"embed"
	"path"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"
	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed compose/snmpCompose.yaml
var snmpCompose string

//go:embed config/public.yaml
var snmpConfig string

const (
	composeDataPath = "compose/data"
)

// snmpDockerStackDef defines a stack with a docker agent on an AmazonLinuxDockerOS VM
// with snmpsim installed and configured with snmp recordings
func snmpDockerStackDef() *e2e.StackDefinition[e2e.DockerEnv] {
	return e2e.EnvFactoryStackDef(func(ctx *pulumi.Context) (*e2e.DockerEnv, error) {
		// setup VM
		vm, err := ec2vm.NewUnixEc2VM(ctx, ec2params.WithOS(ec2os.AmazonLinuxDockerOS))
		if err != nil {
			return nil, err
		}

		fakeintakeExporter, err := aws.NewEcsFakeintake(vm.GetAwsEnvironment())
		if err != nil {
			return nil, err
		}

		filemanager := vm.GetFileManager()
		// upload snmpsim data files
		createDataDirCommand, dataPath, err := filemanager.TempDirectory("data")
		if err != nil {
			return nil, err
		}
		dataFiles, err := loadDataFileNames()
		if err != nil {
			return nil, err
		}

		fileCommands := []pulumi.Resource{}
		for _, fileName := range dataFiles {
			fileContent, err := dataFolder.ReadFile(path.Join(composeDataPath, fileName))
			if err != nil {
				return nil, err
			}
			dontUseSudo := false
			fileCommand, err := filemanager.CopyInlineFile(pulumi.String(fileContent), path.Join(dataPath, fileName), dontUseSudo,
				pulumi.DependsOn([]pulumi.Resource{createDataDirCommand}))
			if err != nil {
				return nil, err
			}
			fileCommands = append(fileCommands, fileCommand)
		}

		createConfigDirCommand, configPath, err := filemanager.TempDirectory("config")
		if err != nil {
			return nil, err
		}
		// edit snmp config file
		dontUseSudo := false
		configCommand, err := filemanager.CopyInlineFile(pulumi.String(snmpConfig), path.Join(configPath, "snmp.yaml"), dontUseSudo,
			pulumi.DependsOn([]pulumi.Resource{createConfigDirCommand}))
		if err != nil {
			return nil, err
		}

		// install agent and snmpsim on docker
		envVars := pulumi.StringMap{"DATA_DIR": pulumi.String(dataPath), "CONFIG_DIR": pulumi.String(configPath)}
		composeDependencies := []pulumi.Resource{createDataDirCommand, configCommand}
		composeDependencies = append(composeDependencies, fileCommands...)
		docker, err := agent.NewDaemon(
			vm,
			dockeragentparams.WithFakeintake(fakeintakeExporter),
			dockeragentparams.WithExtraComposeManifest("snmpsim", snmpCompose),
			dockeragentparams.WithEnvironmentVariables(envVars),
			dockeragentparams.WithPulumiDependsOn(pulumi.DependsOn(composeDependencies)),
		)
		if err != nil {
			return nil, err
		}
		return &e2e.DockerEnv{
			Docker:     client.NewDocker(docker),
			VM:         client.NewPulumiStackVM(vm),
			Fakeintake: client.NewFakeintake(fakeintakeExporter),
		}, nil
	})
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

type snmpDockerSuite struct {
	e2e.Suite[e2e.DockerEnv]
}

// TestSnmpSuite runs the snmp e2e suite
func TestSnmpSuite(t *testing.T) {
	e2e.Run(t, &snmpDockerSuite{}, snmpDockerStackDef())
}

// TestSnmp tests that the snmpsim container is running and that the agent container
// is sending snmp metrics to the fakeintake
func (s *snmpDockerSuite) TestSnmp() {
	fakeintake := s.Env().Fakeintake
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := fakeintake.GetMetricNames()
		assert.NoError(c, err)
		assert.Contains(c, metrics, "snmp.sysUpTimeInstance", "metrics %v doesn't contain snmp.sysUpTimeInstance", metrics)
	}, 5*time.Minute, 10*time.Second)
}
