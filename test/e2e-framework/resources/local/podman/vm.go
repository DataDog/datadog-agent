// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package localpodman

import (
	_ "embed"
	"os"
	"path"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	resourceslocal "github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type VMArgs struct {
	Name string
}

//go:embed data/Dockerfile
var dockerfileContent string
var customDockerConfig = "{}"

func NewInstance(e resourceslocal.Environment, args VMArgs, opts ...pulumi.ResourceOption) (address pulumi.StringOutput, user string, port int, err error) {
	runner := command.NewLocalRunner(&e, command.LocalRunnerArgs{OSCommand: command.NewLocalOSCommand()})
	fileManager := command.NewFileManager(runner)

	publicKey, err := os.ReadFile(e.DefaultPublicKeyPath())
	if err != nil {
		return pulumi.StringOutput{}, "", -1, err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return pulumi.StringOutput{}, "", -1, err
	}
	dataPath := path.Join(homeDir, ".localpodman")
	// TODO clean up the folder on stack destroy
	// Requires a Runner refactor to reuse crossplatform commands
	dataDir, err := fileManager.CreateDirectory(dataPath, false)
	if err != nil {
		return pulumi.StringOutput{}, "", -1, err
	}

	dockerfilePath := path.Join(dataPath, "Dockerfile")
	dockerFile, err := fileManager.CopyInlineFile(pulumi.String(dockerfileContent), dockerfilePath, pulumi.DependsOn([]pulumi.Resource{dataDir}))
	if err != nil {
		return pulumi.StringOutput{}, "", -1, err
	}

	// Use a config to avoid docker hooks that can call vault or other services (credHelpers)
	dockerConfig, err := fileManager.CopyInlineFile(pulumi.String(customDockerConfig), path.Join(dataPath, "config.json"), pulumi.DependsOn([]pulumi.Resource{dataDir}))
	if err != nil {
		return pulumi.StringOutput{}, "", -1, err
	}

	podmanCommand := "podman --config " + dataPath

	opts = utils.MergeOptions(opts, utils.PulumiDependsOn(dockerFile, dockerConfig))
	buildPodman, err := runner.Command("podman-build"+args.Name, &command.LocalArgs{
		Args: command.Args{
			Environment: pulumi.StringMap{"DOCKER_HOST_SSH_PUBLIC_KEY": pulumi.String(string(publicKey))},
			Create:      pulumi.Sprintf("%s build --format=docker --build-arg DOCKER_HOST_SSH_PUBLIC_KEY=\"$DOCKER_HOST_SSH_PUBLIC_KEY\" --build-arg BASE_IMAGE_REGISTRY=669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/ -t %s .", podmanCommand, args.Name),
			Delete:      pulumi.Sprintf("%s rmi %s", podmanCommand, args.Name),
			Triggers:    pulumi.Array{},
		},
		LocalAssetPaths: pulumi.StringArray{},
		LocalDir:        pulumi.String(dataPath),
	}, opts...)
	if err != nil {
		return pulumi.StringOutput{}, "", -1, err
	}
	opts = utils.MergeOptions(opts, utils.PulumiDependsOn(buildPodman))
	runPodman, err := runner.Command("podman-run"+args.Name, &command.LocalArgs{
		Args: command.Args{
			Environment: pulumi.StringMap{"DOCKER_HOST_SSH_PUBLIC_KEY": pulumi.String(string(publicKey))},
			Create:      pulumi.Sprintf("%s run -d --name=%[2]s_run -p 50022:22 %[2]s", podmanCommand, args.Name),
			Delete:      pulumi.Sprintf("%s stop %[2]s_run && podman rm %[2]s_run", podmanCommand, args.Name),
			Triggers:    pulumi.Array{},
		},
		LocalAssetPaths: pulumi.StringArray{},
		LocalDir:        pulumi.String(dataPath),
	}, opts...)
	if err != nil {
		return pulumi.StringOutput{}, "", -1, err
	}

	e.Ctx().Log.Info("Running with container of type ubuntu", nil)

	// hack to wait for the container to be up
	ipAddress := runPodman.StdoutOutput().ApplyT(func(_ string) string {
		return "localhost"
	}).(pulumi.StringOutput)

	return ipAddress, "root", 50022, nil
}
