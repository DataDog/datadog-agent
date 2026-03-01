// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package docker

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	composeVersion = "v2.27.0"
	defaultTimeout = 300
)

type ManagerOutput struct {
	components.JSONImporter

	Host remoteComp.HostOutput `json:"host"`
}

type Manager struct {
	pulumi.ResourceState
	components.Component

	namer namer.Namer
	opts  []pulumi.ResourceOption

	Host *remoteComp.Host `pulumi:"host"`
}

func NewManager(e config.Env, host *remoteComp.Host, opts ...pulumi.ResourceOption) (*Manager, error) {
	return components.NewComponent(e, host.Name(), func(comp *Manager) error {
		comp.namer = e.CommonNamer().WithPrefix("docker")
		comp.Host = host
		comp.opts = opts

		installCmd, err := comp.install()
		if err != nil {
			return err
		}
		comp.opts = utils.MergeOptions(comp.opts, utils.PulumiDependsOn(installCmd))

		composeCmd, err := comp.installCompose()
		if err != nil {
			return err
		}
		comp.opts = utils.MergeOptions(comp.opts, utils.PulumiDependsOn(composeCmd))

		return nil
	}, opts...)
}

func (d *Manager) Export(ctx *pulumi.Context, out *ManagerOutput) error {
	return components.Export(ctx, d, out)
}

func (d *Manager) ComposeFileUp(composeFilePath string, opts ...pulumi.ResourceOption) (command.Command, error) {
	opts = utils.MergeOptions(d.opts, opts...)

	composeHash, err := utils.FileHash(composeFilePath)
	if err != nil {
		return nil, err
	}

	tempCmd, tempDirPath, err := d.Host.OS.FileManager().TempDirectory(composeHash, opts...)
	if err != nil {
		return nil, err
	}
	remoteComposePath := path.Join(tempDirPath, path.Base(composeFilePath))

	copyCmd, err := d.Host.OS.FileManager().CopyFile(filepath.Base(composeFilePath), pulumi.String(composeFilePath), pulumi.String(remoteComposePath), utils.MergeOptions(opts, utils.PulumiDependsOn(tempCmd))...)
	if err != nil {
		return nil, err
	}

	return d.Host.OS.Runner().Command(
		d.namer.ResourceName("run", composeFilePath),
		&command.Args{
			Create: pulumi.Sprintf("docker-compose -f %s up --detach --wait --timeout %d", remoteComposePath, defaultTimeout),
			Delete: pulumi.Sprintf("docker-compose -f %s down -t %d", remoteComposePath, defaultTimeout),
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(copyCmd))...,
	)
}

func (d *Manager) ComposeStrUp(name string, composeManifests []ComposeInlineManifest, envVars pulumi.StringMap, opts ...pulumi.ResourceOption) (command.Command, error) {
	opts = utils.MergeOptions(d.opts, opts...)

	homeCmd, composePath, err := d.Host.OS.FileManager().HomeDirectory(name+"-compose-tmp", opts...)
	if err != nil {
		return nil, err
	}
	var remoteComposePaths []string
	var manifestContents pulumi.StringArray
	runCommandDeps := make([]pulumi.Resource, 0)
	for _, manifest := range composeManifests {
		remoteComposePath := path.Join(composePath, fmt.Sprintf("docker-compose-%s.yml", manifest.Name))
		remoteComposePaths = append(remoteComposePaths, remoteComposePath)

		writeCommand, err := d.Host.OS.FileManager().CopyInlineFile(
			manifest.Content,
			remoteComposePath,
			utils.MergeOptions(d.opts, utils.PulumiDependsOn(homeCmd))...,
		)
		if err != nil {
			return nil, err
		}
		manifestContents = append(manifestContents, manifest.Content)

		runCommandDeps = append(runCommandDeps, writeCommand)
	}
	contentHash := manifestContents.ToStringArrayOutput().ApplyT(func(inputs []string) string {
		mergedContent := strings.Join(inputs, "\n")
		return utils.StrHash(mergedContent)
	}).(pulumi.StringOutput)

	// Initialize envVars if nil to prevent panic
	if envVars == nil {
		envVars = pulumi.StringMap{}
	}

	// We include a hash of the manifests content in the environment variables to trigger an update when a manifest changes
	// This is a workaround to avoid a force replace with Triggers when the content of the manifest changes
	envVars["CONTENT_HASH"] = contentHash

	composeFileArgs := "-f " + strings.Join(remoteComposePaths, " -f ")
	return d.Host.OS.Runner().Command(
		d.namer.ResourceName("compose-run", name),
		&command.Args{
			Create:      pulumi.Sprintf("docker-compose %s up --detach --wait --timeout %d", composeFileArgs, defaultTimeout),
			Delete:      pulumi.Sprintf("docker-compose %s down -t %d", composeFileArgs, defaultTimeout),
			Environment: envVars,
		},
		utils.MergeOptions(d.opts, utils.PulumiDependsOn(runCommandDeps...), pulumi.DeleteBeforeReplace(true))...,
	)
}

func (d *Manager) install() (command.Command, error) {
	opts := []pulumi.ResourceOption{pulumi.Parent(d)}
	opts = utils.MergeOptions(d.opts, opts...)
	dockerInstall, err := d.Host.OS.PackageManager().Ensure("docker", nil, "docker", os.WithPulumiResourceOptions(opts...))
	if err != nil {
		return nil, err
	}

	// Patch ip range that docker uses to create its bridge networks
	// This is to avoid conflicts with other IP ranges used internally
	daemonPatch, err := d.Host.OS.Runner().Command(d.namer.ResourceName("daemon-patch"), &command.Args{
		Create: pulumi.Sprintf("sudo mkdir -p /etc/docker && echo '{\"bip\": \"192.168.16.1/24\", \"default-address-pools\":[{\"base\":\"192.168.32.0/24\", \"size\":24}], \"max-download-attempts\": 10}' | sudo tee /etc/docker/daemon.json"),
		Sudo:   true,
	}, utils.MergeOptions(opts, utils.PulumiDependsOn(dockerInstall))...)
	if err != nil {
		return nil, err
	}

	restartDockerDaemon, err := d.Host.OS.ServiceManger().EnsureRestarted("docker", nil, utils.MergeOptions(opts, utils.PulumiDependsOn(daemonPatch))...)
	if err != nil {
		return nil, err
	}

	whoami, err := d.Host.OS.Runner().Command(
		d.namer.ResourceName("whoami"),
		&command.Args{
			Create: pulumi.String("whoami"),
			Sudo:   false,
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(restartDockerDaemon))...,
	)
	if err != nil {
		return nil, err
	}

	groupCmd, err := d.Host.OS.Runner().Command(
		d.namer.ResourceName("group"),
		&command.Args{
			Create: pulumi.Sprintf("usermod -a -G docker %s", whoami.StdoutOutput()),
			Sudo:   true,
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(whoami))...,
	)
	if err != nil {
		return nil, err
	}

	return groupCmd, err
}

func (d *Manager) LoginRegistry(registry, username string, password pulumi.StringOutput, opts ...pulumi.ResourceOption) (command.Command, error) {
	opts = utils.MergeOptions(d.opts, opts...)
	loginCmd, err := d.Host.OS.Runner().Command(
		d.namer.ResourceName("login", registry),
		&command.Args{
			Create: pulumi.String(fmt.Sprintf(
				"if [ -n \"$REGISTRY_TOKEN\" ]; then echo \"$REGISTRY_TOKEN\" | docker login %s -u %s --password-stdin; fi",
				registry, username,
			)),
			Environment: pulumi.StringMap{
				"REGISTRY_TOKEN": password,
			},
		},
		opts...,
	)
	if err != nil {
		return nil, err
	}
	d.opts = utils.MergeOptions(d.opts, utils.PulumiDependsOn(loginCmd))
	return loginCmd, nil
}

func (d *Manager) installCompose() (command.Command, error) {
	opts := append(d.opts, pulumi.Parent(d))
	installCompose := pulumi.Sprintf("bash -c '(docker-compose version | grep %s) || (curl --retry 10 -fsSLo /usr/local/bin/docker-compose https://github.com/docker/compose/releases/download/%s/docker-compose-linux-$(uname -p) && sudo chmod 755 /usr/local/bin/docker-compose)'", composeVersion, composeVersion)
	return d.Host.OS.Runner().Command(
		d.namer.ResourceName("install-compose"),
		&command.Args{
			Create: installCompose,
			Sudo:   true,
		},
		opts...)
}
