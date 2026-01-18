// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package microvms

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/microvms/resources"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/vmconfig"
)

const DockerMountpoint = "/mnt/docker"

var initSudoPassword sync.Once
var SudoPasswordLocal pulumi.StringOutput
var SudoPasswordRemote pulumi.StringOutput

func GetSudoPassword(ctx *pulumi.Context, isLocal bool) pulumi.StringOutput {
	initSudoPassword.Do(func() {
		rootConfig := config.New(ctx, "")
		SudoPasswordLocal = rootConfig.RequireSecret("sudo-password-local")
		SudoPasswordRemote = rootConfig.RequireSecret("sudo-password-remote")
	})

	if isLocal {
		return SudoPasswordLocal
	}

	return SudoPasswordRemote
}

func setupMicroVMSSHConfig(instance *Instance, subnets map[vmconfig.VMSetID]string, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	createSSHDirArgs := command.Args{
		Create: pulumi.Sprintf("mkdir -p /home/ubuntu/.ssh && chmod 700 /home/ubuntu/.ssh"),
	}
	createDirDone, err := instance.runner.Command(instance.instanceNamer.ResourceName("add-microvm-ssh-dir"), &createSSHDirArgs, pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}

	var config strings.Builder
	for _, subnet := range subnets {
		pattern := getMicroVMGroupSubnetPattern(subnet)
		fmt.Fprintf(&config, "Host %s\nIdentityFile %s\nUser root\nStrictHostKeyChecking no\n", pattern, filepath.Join(GetWorkingDirectory(instance.Arch), "ddvm_rsa"))
	}
	args := command.Args{
		Create: pulumi.Sprintf(`echo -e "%s" | tee /home/ubuntu/.ssh/config && chmod 600 /home/ubuntu/.ssh/config`, config.String()),
	}
	done, err := instance.runner.Command(instance.instanceNamer.ResourceName("add-microvm-ssh-config"), &args, pulumi.DependsOn([]pulumi.Resource{createDirDone}))
	if err != nil {
		return nil, err
	}
	return []pulumi.Resource{done}, nil
}

func readMicroVMSSHKey(instance *Instance, depends []pulumi.Resource) (pulumi.StringOutput, []pulumi.Resource, error) {
	args := command.Args{
		Create: pulumi.Sprintf("cat %s", filepath.Join(GetWorkingDirectory(instance.Arch), "ddvm_rsa")),
	}
	done, err := instance.runner.Command(instance.instanceNamer.ResourceName("read-microvm-ssh-key"), &args, pulumi.DependsOn(depends))
	if err != nil {
		return pulumi.StringOutput{}, nil, err
	}
	s := pulumi.ToSecret(done.StdoutOutput()).(pulumi.StringOutput)
	return s, []pulumi.Resource{done}, err
}

func setupSSHAllowEnv(runner command.Runner, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	allowedEnvVars := []string{"DD_API_KEY", "CI_COMMIT_SHA"}
	args := command.Args{
		Create: pulumi.Sprintf("echo -e 'AcceptEnv %s\n' | sudo tee -a /etc/ssh/sshd_config", strings.Join(allowedEnvVars, " ")),
	}
	done, err := runner.Command("allow-ssh-env", &args, pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}
	return []pulumi.Resource{done}, nil
}

func reloadSSHD(runner command.Runner, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	args := command.Args{
		Create: pulumi.Sprintf("sudo systemctl reload sshd.service"),
	}
	done, err := runner.Command("reload sshd", &args, pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}
	return []pulumi.Resource{done}, nil
}

func mountMicroVMDisks(runner command.Runner, disks []resources.DomainDisk, namer namer.Namer, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	var waitFor []pulumi.Resource

	for _, d := range disks {
		if d.Mountpoint == RootMountpoint {
			continue
		}

		args := command.Args{
			Create: pulumi.Sprintf("mkdir %[1]s && mount %[2]s %[1]s", d.Mountpoint, d.Target),
		}

		done, err := runner.Command(namer.ResourceName("mount-disk", fsPathToLibvirtResource(d.Target)), &args, pulumi.DependsOn(depends))
		if err != nil {
			return nil, err
		}

		waitFor = append(waitFor, done)
	}

	return waitFor, nil
}

func setDockerDataRoot(runner command.Runner, disks []resources.DomainDisk, namer namer.Namer, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	var waitFor []pulumi.Resource

	for _, d := range disks {
		if d.Mountpoint != DockerMountpoint {
			continue
		}

		args := command.Args{
			Create: pulumi.Sprintf("mkdir -p /etc/docker && echo -e '{\n\t\"data-root\":\"%s\"\n}' > /etc/docker/daemon.json && sudo systemctl restart docker", d.Mountpoint),
			Sudo:   true,
		}
		done, err := runner.Command(namer.ResourceName("set-docker-data-root"), &args, pulumi.DependsOn(depends))
		if err != nil {
			return nil, err
		}

		waitFor = append(waitFor, done)

		break
	}

	return waitFor, nil
}

func enableNFSConnKiller(runner command.Runner, namer namer.Namer, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	args := command.Args{
		Create: pulumi.Sprintf("systemctl enable --now kill-dead-nfs-connections.timer"),
		Sudo:   true,
	}
	done, err := runner.Command(namer.ResourceName("enable-nfs-conn-killer"), &args, pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}

	return []pulumi.Resource{done}, nil
}

// This function provisions the metal instance for setting up libvirt based micro-vms.
func provisionMetalInstance(instance *Instance) ([]pulumi.Resource, error) {
	if instance.Arch == LocalVMSet {
		return nil, nil
	}

	allowEnvDone, err := setupSSHAllowEnv(instance.runner, nil)
	if err != nil {
		return nil, err
	}

	reloadSSHDDone, err := reloadSSHD(instance.runner, allowEnvDone)
	if err != nil {
		return nil, err
	}
	return reloadSSHDDone, nil
}

func prepareLibvirtSSHKeys(runner command.Runner, localRunner *command.LocalRunner, resourceNamer namer.Namer, pair sshKeyPair, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	sshGenArgs := command.Args{
		Create: pulumi.Sprintf("rm -f %s && rm -f %s && ssh-keygen -t rsa -b 4096 -f %s -q -N \"\" && cat %s", pair.privateKey, pair.publicKey, pair.privateKey, pair.publicKey),
		Delete: pulumi.Sprintf("rm %s && rm %s", pair.privateKey, pair.publicKey),
	}
	sshgenDone, err := localRunner.Command(resourceNamer.ResourceName("gen-libvirt-sshkey"), &sshGenArgs)
	if err != nil {
		return nil, err
	}

	// This command writes the public ssh key which pulumi uses to talk to the libvirt daemon, in the authorized_keys
	// file of the default user. We must write in this file because pulumi runs its commands as the default user.
	//
	// We override the runner-level user here with root, and construct the path to the default users .ssh directory,
	// in order to write the public ssh key in the correct file.
	sshWriteArgs := command.Args{
		Create: pulumi.Sprintf("echo '%s' >> $(getent passwd 1000 | cut -d: -f6)/.ssh/authorized_keys", sshgenDone.StdoutOutput()),
		Sudo:   true,
	}

	wait := append(depends, sshgenDone)
	sshWrite, err := runner.Command("write-ssh-key", &sshWriteArgs, pulumi.DependsOn(wait))
	if err != nil {
		return nil, err
	}
	return []pulumi.Resource{sshWrite}, nil
}

func provisionRemoteMicroVMs(vmCollections []*VMCollection, instanceEnv *InstanceEnvironment) ([]pulumi.Resource, error) {
	var waitFor []pulumi.Resource

	for _, collection := range vmCollections {
		if collection.instance.Arch == LocalVMSet {
			continue
		}

		sshConfigDone, err := setupMicroVMSSHConfig(collection.instance, collection.subnets, waitFor)
		if err != nil {
			return nil, err
		}

		microVMSSHKey, readKeyDone, err := readMicroVMSSHKey(collection.instance, sshConfigDone)
		if err != nil {
			return nil, err
		}

		for _, domains := range collection.domains {
			for _, domain := range domains {
				if domain.lvDomain == nil {
					continue
				}

				// create new ssh connection to build proxy
				conn, err := remoteComp.NewConnection(
					collection.instance.instance.Address,
					"ubuntu",
					remoteComp.WithPrivateKeyPath(instanceEnv.DefaultPrivateKeyPath()),
					remoteComp.WithPrivateKeyPassword(instanceEnv.DefaultPrivateKeyPassword()),
				)
				if err != nil {
					return nil, err
				}

				pc := createProxyConnection(domain.ip, "root", microVMSSHKey, conn.ToConnectionOutput())
				remoteRunner, err := command.NewRemoteRunner(
					collection.instance.e,
					command.RemoteRunnerArgs{
						ParentResource: domain.lvDomain,
						Connection:     pc,
						ConnectionName: collection.instance.instanceNamer.ResourceName("conn", domain.domainID),
						OSCommand:      command.NewUnixOSCommand(),
					},
				)
				if err != nil {
					return nil, err
				}

				mountDisksDone, err := mountMicroVMDisks(remoteRunner, domain.Disks, domain.domainNamer, []pulumi.Resource{domain.lvDomain})
				if err != nil {
					return nil, err
				}

				setDockerDataRootDone, err := setDockerDataRoot(remoteRunner, domain.Disks, domain.domainNamer, mountDisksDone)
				if err != nil {
					return nil, err
				}

				deps := append(readKeyDone, setDockerDataRootDone...)
				deps = append(deps, domain.lvDomain)
				reloadSSHDDone, err := reloadSSHD(remoteRunner, deps)
				if err != nil {
					return nil, err
				}

				waitFor = append(waitFor, reloadSSHDDone...)
			}
		}
	}

	return waitFor, nil
}

func provisionLocalMicroVMs(vmCollections []*VMCollection) ([]pulumi.Resource, error) {
	var waitFor []pulumi.Resource
	for _, collection := range vmCollections {
		if collection.instance.Arch != LocalVMSet {
			continue
		}

		for _, dls := range collection.domains {
			for _, domain := range dls {
				if domain.lvDomain == nil {
					continue
				}

				// create new ssh connection to build proxy
				conn, err := remoteComp.NewConnection(
					domain.ip,
					"root",
					remoteComp.WithPrivateKeyPath(filepath.Join(GetWorkingDirectory(domain.vmset.Arch), "ddvm_rsa")),
				)
				if err != nil {
					return nil, err
				}

				remoteRunner, err := command.NewRemoteRunner(
					*collection.instance.e,
					command.RemoteRunnerArgs{
						ParentResource: domain.lvDomain,
						Connection:     conn,
						ConnectionName: domain.domainNamer.ResourceName("provision-conn"),
						OSCommand:      command.NewUnixOSCommand(),
					},
				)
				if err != nil {
					return nil, err
				}

				if collection.instance.IsMacOSHost() {
					nfsConnKillerDone, err := enableNFSConnKiller(remoteRunner, domain.domainNamer, []pulumi.Resource{domain.lvDomain})
					if err != nil {
						return nil, err
					}
					waitFor = append(waitFor, nfsConnKillerDone...)
				}

				mountDisksDone, err := mountMicroVMDisks(remoteRunner, domain.Disks, domain.domainNamer, []pulumi.Resource{domain.lvDomain})
				if err != nil {
					return nil, err
				}

				setDockerDataRootDone, err := setDockerDataRoot(remoteRunner, domain.Disks, domain.domainNamer, mountDisksDone)
				if err != nil {
					return nil, err
				}

				waitFor = append(waitFor, setDockerDataRootDone...)
			}
		}
	}

	return waitFor, nil
}
