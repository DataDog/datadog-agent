// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package docker

import (
	"fmt"
	"maps"
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

	namer          namer.Namer
	opts           []pulumi.ResourceOption
	defaultEnvVars pulumi.StringMap

	Host *remoteComp.Host `pulumi:"host"`
}

func NewManager(e config.Env, host *remoteComp.Host, opts ...pulumi.ResourceOption) (*Manager, error) {
	return components.NewComponent(e, host.Name(), func(comp *Manager) error {
		comp.namer = e.CommonNamer().WithPrefix("docker")
		comp.Host = host
		comp.opts = opts

		prepareCmd, err := comp.prepare()
		if err != nil {
			return err
		}
		comp.opts = utils.MergeOptions(comp.opts, utils.PulumiDependsOn(prepareCmd))

		composeCmd, err := comp.assertCompose()
		if err != nil {
			return err
		}
		comp.opts = utils.MergeOptions(comp.opts, utils.PulumiDependsOn(composeCmd))

		// Inject the internal registry hosting the workload app images so that
		// compose files using ${DD_APPS_REGISTRY:-ghcr.io/datadog} pull the apps
		// from the internal registry (agent-qa ECR, GCP Artifact Registry, Azure
		// ACR depending on the cloud) when one is available. Environments without
		// an internal registry (local runs, which report "none") keep the
		// default public ghcr.io.
		if reg := e.InternalRegistry(); reg != "" && reg != "none" {
			comp.defaultEnvVars = pulumi.StringMap{
				"DD_APPS_REGISTRY": pulumi.String(reg),
			}
		}

		return nil
	}, opts...)
}

// NewAWSManager creates a docker Manager and wires the host's Amazon ECR
// credentials helper so it authenticates automatically against ECR registries
// (including our pull-through cache). The helper itself is pre-baked into the
// AWS e2e AMI — NewAWSManager only writes ~/.docker/config.json via
// SetupECRDockerAuth. Use this instead of NewManager when the host is on AWS.
//
// When ImagePullRegistry is configured, DD_REGISTRY is automatically injected into every
// ComposeStrUp call so that compose files using ${DD_REGISTRY:-docker.io} pull from the
// ECR pull-through cache for Docker Hub images. Callers may still override DD_REGISTRY by
// passing it explicitly in their envVars map. DD_APPS_REGISTRY (the internal registry
// hosting the workload app images, set by NewManager) is injected the same way so compose
// files using ${DD_APPS_REGISTRY:-ghcr.io/datadog} pull the workload apps from ECR.
func NewAWSManager(e config.Env, host *remoteComp.Host, opts ...pulumi.ResourceOption) (*Manager, error) {
	ecrCreds, err := SetupECRDockerAuth(e.CommonNamer().WithPrefix("docker"), host, opts...)
	if err != nil {
		return nil, err
	}
	mgr, err := NewManager(e, host, utils.MergeOptions(opts, utils.PulumiDependsOn(ecrCreds))...)
	if err != nil {
		return nil, err
	}
	if reg := e.ImagePullRegistry(); reg != "" {
		if mgr.defaultEnvVars == nil {
			mgr.defaultEnvVars = pulumi.StringMap{}
		}
		mgr.defaultEnvVars["DD_REGISTRY"] = pulumi.String(strings.SplitN(reg, ",", 2)[0] + "/dockerhub")
	}
	return mgr, nil
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

	// Merge defaultEnvVars (set at manager construction time) with caller-provided envVars.
	// Caller-provided values take precedence.
	merged := pulumi.StringMap{}
	maps.Copy(merged, d.defaultEnvVars)
	maps.Copy(merged, envVars)
	envVars = merged

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

func (d *Manager) prepare() (command.Command, error) {
	opts := []pulumi.ResourceOption{pulumi.Parent(d)}
	opts = utils.MergeOptions(d.opts, opts...)

	// AWS images bake Docker; other clouds provision it explicitly before NewManager.
	dockerCheck, err := d.Host.OS.PackageManager().AssertInstalled("docker", os.WithPulumiResourceOptions(opts...))
	if err != nil {
		return nil, err
	}
	opts = utils.MergeOptions(opts, utils.PulumiDependsOn(dockerCheck))

	// Retain host configuration that used to accompany the RPM installation.
	switch d.Host.OS.Descriptor().Flavor {
	case os.RedHat, os.CentOS, os.RockyLinux, os.AlmaLinux, os.AmazonLinux:
		configure, err := d.Host.OS.Runner().Command(d.namer.ResourceName("docker-host-config"), &command.Args{
			Sudo: true,
			Create: pulumi.String(`bash <<'EOF'
set -euxo pipefail
setenforce 0 || true
sed -i 's/^SELINUX=enforcing/SELINUX=permissive/' /etc/selinux/config || true
systemctl disable --now firewalld || true
echo 'net.ipv4.ip_forward=1' > /etc/sysctl.d/99-docker.conf
sysctl -w net.ipv4.ip_forward=1
systemctl enable docker
EOF`),
		}, opts...)
		if err != nil {
			return nil, err
		}
		opts = utils.MergeOptions(opts, utils.PulumiDependsOn(configure))
	}

	// Patch the daemon config: pin overlay2 + a mirror and move docker off the
	// default bridge IP range to avoid conflicts with other internal ranges.
	// Red Hat 10 dropped the legacy iptables kernel module, so docker 29 needs
	// its nftables firewall backend or the daemon cannot program bridge NAT.
	daemonOpts := `"storage-driver": "overlay2", "registry-mirrors": ["https://mirror.gcr.io"], "bip": "192.168.16.1/24", "default-address-pools":[{"base":"192.168.32.0/24", "size":24}], "max-download-attempts": 10`
	desc := d.Host.OS.Descriptor()
	majorVersion := strings.Split(desc.Version, "-")[0]
	if (desc.Flavor == os.RedHat || desc.Flavor == os.AlmaLinux || desc.Flavor == os.RockyLinux) &&
		(majorVersion == "9" || majorVersion == "10") {
		daemonOpts += `, "firewall-backend": "nftables"`
	}
	daemonPatch, err := d.Host.OS.Runner().Command(d.namer.ResourceName("daemon-patch"), &command.Args{
		Create: pulumi.Sprintf("sudo mkdir -p /etc/docker && echo '{%s}' | sudo tee /etc/docker/daemon.json", daemonOpts),
		Sudo:   true,
	}, opts...)
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

// assertCompose verifies that docker-compose at composeVersion is already
// present on the host. Actual installs happen either via the pre-baked AWS e2e
// AMI or via docker.InstallCompose, called explicitly by Azure/GCP
// provisioners before NewManager. This method never installs — runtime
// installs on AWS are disallowed.
//
// Version validation runs in Go (via ApplyT) so a missing or wrong version
// surfaces as a typed Go error rather than a bare bash exit code.
func (d *Manager) assertCompose() (command.Command, error) {
	opts := append(d.opts, pulumi.Parent(d))

	versionCmd, err := d.Host.OS.Runner().Command(
		d.namer.ResourceName("compose-version"),
		&command.Args{
			Create: pulumi.String("docker-compose version"),
			Sudo:   false,
		},
		opts...)
	if err != nil {
		return nil, err
	}

	validated := versionCmd.StdoutOutput().ApplyT(func(out string) (string, error) {
		if !strings.Contains(out, composeVersion) {
			return "", fmt.Errorf(
				"docker-compose %s expected on host but got %q; runtime installs are not allowed on AWS — use docker.InstallCompose on Azure/GCP",
				composeVersion, strings.TrimSpace(out),
			)
		}
		return ":", nil
	}).(pulumi.StringOutput)

	return d.Host.OS.Runner().Command(
		d.namer.ResourceName("assert-compose"),
		&command.Args{
			Create: validated,
			Sudo:   false,
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(versionCmd))...)
}
