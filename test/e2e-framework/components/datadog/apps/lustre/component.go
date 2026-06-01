// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package lustre provides an Agent E2E component that bootstraps an all-in-one
// Lustre 2.15 filesystem (MGS/MGT + MDS/MDT + OSS/OST + client) on a single
// x86_64 EL9 host, and runs a continuous I/O + metadata workload against the
// client mount so that the Datadog `lustre` integration produces non-empty
// metrics for the client, mds and oss node types.
//
// Lustre relies on out-of-tree kernel modules (lustre, lnet, ldiskfs, ...) and
// a kernel-matched, Lustre-patched e2fsprogs (mkfs.lustre). It therefore cannot
// be represented faithfully by Docker Compose; the honest deployment is the
// upstream `llmount.sh`-style single-node dev shape over loopback LNet
// (`tcp0` / `0@lo`) with loop-device backing files for the targets.
//
// This component owns the host bootstrap and asset copy. The scenario `run.go`
// (downstream phase) is responsible for creating the EC2 VM, installing the
// Datadog Agent with the three-instance `lustre.d/conf.yaml`, and ordering the
// Agent install after this component reports the filesystem healthy.
package lustre

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Filesystem-wide constants shared between the component and the embedded
// scripts. The check config (config/conf.yaml) pins `filesystems: [lustre]`,
// so the fsname here MUST stay in sync with that pin.
const (
	// FilesystemName is the Lustre fsname created by the bootstrap. It is also
	// the value pinned in `filesystems:` in config/conf.yaml.
	FilesystemName = "lustre"

	// ClientMountPoint is where the all-in-one client mount lives; the load
	// script writes exclusively under this path.
	ClientMountPoint = "/mnt/lustre"

	// remoteScriptDir is the absolute directory where the embedded scripts are
	// staged on the host. /tmp is chosen (not $HOME or /opt) so the SSH user can
	// create it and write to it without sudo, while the absolute path expands
	// identically whether the script later runs as the SSH user or under sudo.
	remoteScriptDir = "/tmp/lustre-lab"

	// LustreVersion is the Lustre series targeted (>= 2.15.5 for the `-y` YAML
	// path the check prefers). The install script resolves the concrete el9
	// point release that matches the running kernel's EL minor.
	LustreVersion = "2.15"
)

// Embedded host-side assets. All assets referenced by this component are
// embedded explicitly so the asset-copy behavior is part of the component and
// does not depend on files existing in any particular working directory.
var (
	//go:embed scripts/install.sh
	installScript string

	//go:embed scripts/configure.sh
	configureScript string

	//go:embed scripts/load.sh
	loadScript string

	//go:embed fixtures/lustre-load.service
	loadServiceUnit string

	//go:embed fixtures/lustre.sudoers
	sudoersDropin string
)

// LustreHost is the Pulumi component that turns a bare EL9 host into a working
// all-in-one Lustre filesystem with a running I/O workload.
type LustreHost struct {
	pulumi.ResourceState
	components.Component

	namer namer.Namer
	host  *remoteComp.Host

	// Ready is a resource that completes only once the filesystem is mounted
	// and healthy and the workload is running. The scenario should gate the
	// Datadog Agent install on this via utils.PulumiDependsOn(comp.Ready).
	Ready pulumi.Resource
}

// NewLustreHost provisions the full Lustre bootstrap on the given host and
// returns the component. `host` is the EC2 VM created by the scenario.
//
// The bootstrap is split into idempotent, ordered phases so it can be
// re-run safely while iterating on a live host:
//
//  1. install.sh   - Whamcloud server+client repos, Lustre-patched e2fsprogs,
//     lustre-dkms + matching kernel-devel, kernel versionlock,
//     then a reboot into the pinned kernel (the DKMS modules are
//     built against it). Idempotent: skips when modules already
//     load. Reboots only when a reboot is required.
//  2. configure.sh - modprobe lustre/lnet, configure LNet (tcp0 over lo),
//     mkfs.lustre the MGT/MDT/OST on loop-backed files, mount
//     MGS -> MDT -> OST -> client at /mnt/lustre, enable jobstats
//     (jobid_var=procname_uid), register a changelog user, install
//     the dd-agent NOPASSWD sudoers drop-in, install + start the
//     load systemd unit, and run a warm-up I/O pass so counters are
//     already moving before the first Agent check.
//
// The reboot in phase 1 makes this a genuine two-phase flow. configure.sh is
// ordered strictly after install.sh via PulumiDependsOn; install.sh exits 0
// once the host is in the desired pre-configure state (modules buildable and
// the pinned kernel running), triggering its own reboot when needed and
// blocking until SSH is reachable again on the next Pulumi command.
func NewLustreHost(e config.Env, name string, host *remoteComp.Host, opts ...pulumi.ResourceOption) (*LustreHost, error) {
	return components.NewComponent(e, name, func(comp *LustreHost) error {
		comp.namer = e.CommonNamer().WithPrefix(comp.Name())
		comp.host = host

		runner := host.OS.Runner()
		fm := command.NewFileManager(runner)

		// Stage all host-side assets in an absolute directory the SSH user can
		// create and write to without sudo. The scripts then run with sudo; the
		// absolute path expands identically in both cases.
		dirCmd, err := fm.CreateDirectory(remoteScriptDir, false)
		if err != nil {
			return err
		}

		// Copy each asset inline so the component is self-contained and does not
		// depend on the caller's working directory.
		installCopy, err := fm.CopyInlineFile(
			pulumi.String(installScript),
			remoteScriptDir+"/install.sh",
			utils.PulumiDependsOn(dirCmd),
		)
		if err != nil {
			return err
		}
		configureCopy, err := fm.CopyInlineFile(
			pulumi.String(configureScript),
			remoteScriptDir+"/configure.sh",
			utils.PulumiDependsOn(installCopy),
		)
		if err != nil {
			return err
		}
		loadCopy, err := fm.CopyInlineFile(
			pulumi.String(loadScript),
			remoteScriptDir+"/load.sh",
			utils.PulumiDependsOn(configureCopy),
		)
		if err != nil {
			return err
		}
		serviceCopy, err := fm.CopyInlineFile(
			pulumi.String(loadServiceUnit),
			remoteScriptDir+"/lustre-load.service",
			utils.PulumiDependsOn(loadCopy),
		)
		if err != nil {
			return err
		}
		// The dd-agent NOPASSWD sudoers drop-in is installed into
		// /etc/sudoers.d by configure.sh (it must land with 0440 perms and pass
		// visudo), so stage it under the script dir first.
		sudoersCopy, err := fm.CopyInlineFile(
			pulumi.String(sudoersDropin),
			remoteScriptDir+"/lustre.sudoers",
			utils.PulumiDependsOn(serviceCopy),
		)
		if err != nil {
			return err
		}

		assetDeps := []pulumi.Resource{installCopy, configureCopy, loadCopy, serviceCopy, sudoersCopy}

		// Phase 1: install + kernel pin + reboot.
		install, err := runner.Command(
			comp.namer.ResourceName("install"),
			&command.Args{
				Create: pulumi.Sprintf(
					"LUSTRE_VERSION=%s bash %s/install.sh",
					LustreVersion, remoteScriptDir,
				),
				Sudo: true,
			},
			utils.PulumiDependsOn(assetDeps...),
		)
		if err != nil {
			return err
		}

		// Phase 2: configure (modprobe -> LNet -> mkfs.lustre -> mount ->
		// jobstats -> changelog -> sudoers -> load service -> warm-up).
		configure, err := runner.Command(
			comp.namer.ResourceName("configure"),
			&command.Args{
				Create: pulumi.Sprintf(
					"FSNAME=%s MOUNT_POINT=%s SCRIPT_DIR=%s bash %s/configure.sh",
					FilesystemName, ClientMountPoint, remoteScriptDir, remoteScriptDir,
				),
				// Delete tears the workload + mounts down on `pulumi destroy`
				// before the EC2 instance is removed. Best-effort; the instance
				// is ephemeral so this is belt-and-suspenders.
				Delete: pulumi.Sprintf(
					"SCRIPT_DIR=%s MOUNT_POINT=%s bash %s/configure.sh teardown || true",
					remoteScriptDir, ClientMountPoint, remoteScriptDir,
				),
				Sudo: true,
			},
			utils.PulumiDependsOn(install),
		)
		if err != nil {
			return err
		}

		comp.Ready = configure
		return nil
	}, opts...)
}
