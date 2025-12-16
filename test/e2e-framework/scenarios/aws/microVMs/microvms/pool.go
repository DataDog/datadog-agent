// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package microvms

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/microvms/resources"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/vmconfig"
	"github.com/pulumi/pulumi-libvirt/sdk/go/libvirt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type LibvirtPool interface {
	SetupLibvirtPool(ctx *pulumi.Context, runner command.Runner, providerFn LibvirtProviderFn, isLocal bool, depends []pulumi.Resource) ([]pulumi.Resource, error)
	Name() string
	Type() vmconfig.PoolType
	Path() string
}

type globalLibvirtPool struct {
	poolName    string
	poolXML     pulumi.StringOutput
	poolXMLPath string
	poolNamer   namer.Namer
	poolType    vmconfig.PoolType
	poolPath    string
}

func generateGlobalPoolPath(name string) string {
	return fmt.Sprintf("%s/libvirt/pools/%s", GetWorkingDirectory(LocalVMSet), name)
}

func NewGlobalLibvirtPool(ctx *pulumi.Context) LibvirtPool {
	poolName := libvirtResourceName(ctx.Stack(), "global-pool")
	rc := resources.NewResourceCollection(vmconfig.RecipeDefault)
	poolPath := generateGlobalPoolPath(poolName)
	poolXML := rc.GetPoolXML(
		map[string]pulumi.StringInput{
			resources.PoolName: pulumi.String(poolName),
			resources.PoolPath: pulumi.String(poolPath),
		},
	)

	return &globalLibvirtPool{
		poolName:    poolName,
		poolXML:     poolXML,
		poolXMLPath: fmt.Sprintf("/tmp/pool-%s.tmp", poolName),
		poolNamer:   libvirtResourceNamer(ctx, poolName),
		poolType:    resources.DefaultPool,
		poolPath:    poolPath,
	}
}

/*
Setup for remote pool and local pool is different for a number of reasons:
  - Libvirt pools and volumes on remote machines are setup using the virsh cli tool. This is because
    the pulumi-libvirt sdk always uploads the base volume image from the host (where pulumi runs) to the
    remote machine (where the micro-vms are setup).
    This is too inefficient for us. We would like for it to assume the images are already present on the remote
    machine. Therefore we create volumes using the virsh cli and we have to create the pools in the same way
    since we cannot pass the `pool` object, returned by the pulumi-libvirt api,  around in remote commands.
  - On the remote machine all commands are run with 'sudo' to simplify permission issues;
    we do not want to do this on the local machine. For local machines the pulumi-libvirt API works fine, since
    the target environment and the pulumi host are the same machine. It is simpler to use this API locally than
    have a complicated permissions setup.
*/
func remoteGlobalPool(p *globalLibvirtPool, runner command.Runner, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	poolXMLWrittenArgs := command.Args{
		Create: pulumi.Sprintf("echo \"%s\" > %s", p.poolXML, p.poolXMLPath),
		Delete: pulumi.Sprintf("rm -f %s", p.poolXMLPath),
	}
	poolXMLWritten, err := runner.Command(p.poolNamer.ResourceName("write-pool-xml"), &poolXMLWrittenArgs, pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}
	depends = append(depends, poolXMLWritten)

	poolBuildReadyArgs := command.Args{
		Create: pulumi.Sprintf("virsh pool-build %s", p.poolName),
		Delete: pulumi.Sprintf("virsh pool-delete %s", p.poolName),
		Sudo:   true,
	}
	poolStartReadyArgs := command.Args{
		Create: pulumi.Sprintf("virsh pool-start %s", p.poolName),
		Delete: pulumi.Sprintf("virsh pool-destroy %s", p.poolName),
		Sudo:   true,
	}
	poolRefreshDoneArgs := command.Args{
		Create: pulumi.Sprintf("virsh pool-refresh %s", p.poolName),
		Sudo:   true,
	}

	poolDefineReadyArgs := command.Args{
		Create: pulumi.Sprintf("virsh pool-define %s", p.poolXMLPath),
		Sudo:   true,
	}

	poolDefineReady, err := runner.Command(p.poolNamer.ResourceName("define-libvirt-pool"), &poolDefineReadyArgs, pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}

	poolBuildReady, err := runner.Command(p.poolNamer.ResourceName("build-libvirt-pool"), &poolBuildReadyArgs, pulumi.DependsOn([]pulumi.Resource{poolDefineReady}))
	if err != nil {
		return nil, err
	}

	poolStartReady, err := runner.Command(p.poolNamer.ResourceName("start-libvirt-pool"), &poolStartReadyArgs, pulumi.DependsOn([]pulumi.Resource{poolBuildReady}))
	if err != nil {
		return nil, err
	}

	poolRefreshDone, err := runner.Command(p.poolNamer.ResourceName("refresh-libvirt-pool"), &poolRefreshDoneArgs, pulumi.DependsOn([]pulumi.Resource{poolStartReady}))
	if err != nil {
		return nil, err
	}

	return []pulumi.Resource{poolRefreshDone}, nil
}

func localGlobalPool(ctx *pulumi.Context, p *globalLibvirtPool, providerFn LibvirtProviderFn, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	provider, err := providerFn()
	if err != nil {
		return nil, err
	}

	poolReady, err := libvirt.NewPool(ctx, "create-libvirt-pool", &libvirt.PoolArgs{
		Type: pulumi.String("dir"),
		Name: pulumi.String(p.poolName),
		Path: pulumi.String(p.poolPath),
	}, pulumi.Provider(provider), pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}

	return []pulumi.Resource{poolReady}, nil
}

func (p *globalLibvirtPool) SetupLibvirtPool(ctx *pulumi.Context, runner command.Runner, providerFn LibvirtProviderFn, isLocal bool, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	if isLocal {
		return localGlobalPool(ctx, p, providerFn, depends)
	}

	return remoteGlobalPool(p, runner, depends)
}

func (p *globalLibvirtPool) Name() string {
	return p.poolName
}

func (p *globalLibvirtPool) Type() vmconfig.PoolType {
	return p.poolType
}

func (p *globalLibvirtPool) Path() string {
	return p.poolPath
}

type rambackedLibvirtPool struct {
	poolName      string
	poolNamer     namer.Namer
	poolType      vmconfig.PoolType
	poolPath      string
	poolSize      string
	baseImagePath string
}

const sharedDiskCmd = `MYUSER=$(id -u) MYGROUP=$(id -g) sh -c \
'mkdir -p %[1]s && \
sudo -E -S mount -t ramfs -o size=%[2]s,uid=$MYUSER,gid=$MYGROUP,othmask=0077,mode=0777 ramfs %[1]s && \
mkdir %[1]s/deps && \
dd if=/dev/zero of=%[3]s bs=%[2]s count=1 && \
mkfs.ext4 -F %[3]s' && \
sudo mount -o exec,loop %[3]s %[1]s/deps && cd %[1]s/deps && sudo touch win.123 && cd %[1]s && sudo umount %[1]s/deps \
`

func NewRAMBackedLibvirtPool(ctx *pulumi.Context, disk *vmconfig.Disk) (LibvirtPool, error) {
	poolName := libvirtResourceName(ctx.Stack(), "ram-pool")
	baseImagePath := strings.TrimPrefix(disk.BackingStore, "file://")
	poolPath := filepath.Dir(baseImagePath)

	if disk.Size == "" {
		return nil, ErrZeroRAMDiskSize
	}

	if !(strings.HasSuffix(disk.Size, "G") || strings.HasSuffix(disk.Size, "M")) {
		return nil, ErrInvalidPoolSize
	}

	return &rambackedLibvirtPool{
		poolName:      poolName,
		poolNamer:     libvirtResourceNamer(ctx, poolName),
		poolType:      resources.RAMPool,
		poolPath:      poolPath,
		poolSize:      disk.Size,
		baseImagePath: baseImagePath,
	}, nil
}

func (p *rambackedLibvirtPool) SetupLibvirtPool(ctx *pulumi.Context, runner command.Runner, providerFn LibvirtProviderFn, isLocal bool, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	buildSharedDiskInRamfsArgs := command.Args{
		Create: pulumi.Sprintf(sharedDiskCmd, p.poolPath, p.poolSize, p.baseImagePath),
		Delete: pulumi.Sprintf("umount %[1]s && rm -r %[1]s", p.poolPath),
		Stdin:  GetSudoPassword(ctx, isLocal),
	}

	buildSharedDiskInRamfsDone, err := runner.Command("build-shared-disk", &buildSharedDiskInRamfsArgs, pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}

	provider, err := providerFn()
	if err != nil {
		return nil, err
	}

	poolReady, err := libvirt.NewPool(ctx, p.poolNamer.ResourceName("create-libvirt-ram-pool"), &libvirt.PoolArgs{
		Type: pulumi.String("dir"),
		Name: pulumi.String(p.poolName),
		Path: pulumi.String(p.poolPath),
	}, pulumi.Provider(provider), pulumi.DependsOn([]pulumi.Resource{buildSharedDiskInRamfsDone}))
	if err != nil {
		return nil, err
	}
	return []pulumi.Resource{poolReady}, nil
}

func (p *rambackedLibvirtPool) Name() string {
	return p.poolName
}

func (p *rambackedLibvirtPool) Type() vmconfig.PoolType {
	return p.poolType
}

func (p *rambackedLibvirtPool) Path() string {
	return p.poolPath
}
