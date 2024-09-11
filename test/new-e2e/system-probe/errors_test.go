// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package systemprobe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const outputLocalError = `
Updating (gjulian-guillermo.julian-e2e-report-all-errors-ddvm):

    pulumi:pulumi:Stack e2elocal-gjulian-guillermo.julian-e2e-report-all-errors-ddvm running
    pulumi:providers:random random
@ updating....
    dd:Host aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64
    pulumi:providers:aws aws
    pulumi:providers:command command
    random:index:RandomShuffle aws-rnd-subnet
    random:index:RandomString random-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-random-string-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192
    command:local:Command local-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-gen-libvirt-sshkey
    aws:ec2:Instance aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-wait-cloud-init
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-write-ssh-key
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-ubuntu_22.04-distro_arm64-arm64-write-vol-xml
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-add-microvm-ssh-dir
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-allow-ssh-env
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-distro_arm64-download-with-curl
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-docker-arm64.qcow2-distro_arm64-arm64-write-vol-xml
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-add-microvm-ssh-config
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-reload sshd
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-docker-arm64.qcow2-distro_arm64-arm64-extract-base-volume-package
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-ubuntu_22.04-distro_arm64-arm64-extract-base-volume-package
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-read-microvm-ssh-key
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-write-pool-xml
    pulumi:providers:libvirt gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-libvirt-provider
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-define-libvirt-pool
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-build-libvirt-pool
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-start-libvirt-pool
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-refresh-libvirt-pool
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-docker-arm64.qcow2-distro_arm64-arm64-build-libvirt-basevolume
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-ubuntu_22.04-distro_arm64-arm64-build-libvirt-basevolume
@ updating.....
    libvirt:index:Volume gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-ubuntu_22.04-distro_arm64-arm64-overlay-ubuntu_22.04-4-8192
    libvirt:index:Volume gjulian-guillermo.julian-e2e-report-all-errors-ddvm-global-pool-docker-arm64.qcow2-distro_arm64-arm64-overlay-ubuntu_22.04-4-8192
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-create-nvram
    libvirt:index:Network gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-network-distro_arm64-arm64
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-allow-nfs-ports-tcp-100.1.0.0/24
    libvirt:index:Domain arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192
    command:remote:Command remote-aws-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-cmd-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-allow-nfs-ports-udp-100.1.0.0/24
 +  command:remote:Command remote-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-conn-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-cmd-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-mount-disk-dev-vdb creating (0s)
@ updating.....
 +  command:remote:Command remote-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-conn-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-cmd-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-mount-disk-dev-vdb creating (2s) bash: line 1: caca: command not found
 +  command:remote:Command remote-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-conn-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-cmd-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-mount-disk-dev-vdb creating (2s) error: Process exited with status 127: running " caca /mnt/docker && mount /dev/vdb /mnt/docker":
 +  command:remote:Command remote-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-conn-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-cmd-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-mount-disk-dev-vdb **creating failed** error: Process exited with status 127: running " caca /mnt/docker && mount /dev/vdb /mnt/docker":
    pulumi:pulumi:Stack e2elocal-gjulian-guillermo.julian-e2e-report-all-errors-ddvm running error: update failed
    pulumi:pulumi:Stack e2elocal-gjulian-guillermo.julian-e2e-report-all-errors-ddvm **failed** 1 error
Diagnostics:
  command:remote:Command (remote-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-conn-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-cmd-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-mount-disk-dev-vdb):
    error: Process exited with status 127: running " nocommand /mnt/docker && mount /dev/vdb /mnt/docker":
    bash: line 1: nocommand: command not found

  pulumi:pulumi:Stack (e2elocal-gjulian-guillermo.julian-e2e-report-all-errors-ddvm):
    error: update failed

Outputs:
    kmt-stack: (json) {
        arm64: {
            ip      : "172.29.176.14"
            microvms: [
                [0]: {
                    id          : "arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192"
                    ip          : "100.1.0.2"
                    ssh-key-path: "/home/kernel-version-testing/ddvm_rsa"
                    tag         : "ubuntu_22.04"
                    vmset-tags  : [
                        [0]: "distro_arm64"
                    ]
                }
            ]
        }
    }


Resources:
    36 unchanged

Duration: 6s
`

const outputSshFailed = `
    pulumi:pulumi:Stack e2eci-ci-630160752-4670-kernel-matrix-testing-system-probe-arm64-43724877 **failed** 1 error
Diagnostics:
  pulumi:pulumi:Stack (e2eci-ci-630160752-4670-kernel-matrix-testing-system-probe-arm64-43724877):
    error: update failed
  command:remote:Command (remote-ci-630160752-4670-kernel-matrix-testing-system-probe-arm64-43724877-arm64-conn-arm64-fedora_37-no_usm-distro_arm64-ddvm-4-12288-cmd-arm64-fedora_37-no_usm-distro_arm64-ddvm-4-12288-mount-disk-dev-vdb):
    error: proxy: after 60 failed attempts: ssh: rejected: connect failed (No route to host)
Outputs:
    kmt-stack: [secret]
Resources:
    +-8 replaced
    349 unchanged
Duration: 7m35s
`

const outputSshFailedWithChangedOrder = `
@ updating....
 +  pulumi:pulumi:Stack e2eci-ci-630160752-4670-kernel-matrix-testing-system-probe-arm64-43724877 creating (933s) error: update failed
 +  pulumi:pulumi:Stack e2eci-ci-630160752-4670-kernel-matrix-testing-system-probe-arm64-43724877 **creating failed (933s)** 1 error
Diagnostics:
  command:remote:Command (remote-ci-630160752-4670-kernel-matrix-testing-system-probe-arm64-43724877-arm64-conn-arm64-fedora_37-no_usm-distro_arm64-ddvm-4-12288-cmd-arm64-fedora_37-no_usm-distro_arm64-ddvm-4-12288-mount-disk-dev-vdb):
    error: proxy: after 60 failed attempts: ssh: rejected: connect failed (No route to host)

  pulumi:pulumi:Stack (e2eci-ci-630160752-4670-kernel-matrix-testing-system-probe-arm64-43724877):
    error: update failed

Outputs:
    kmt-stack: [secret]

Resources:
    + 357 created

Duration: 15m34s
`

func TestParseDiagnostics(t *testing.T) {
	cases := []struct {
		caseName string
		output   string
		result   pulumiError
	}{
		{
			caseName: "LocalError",
			output:   outputLocalError,
			result: pulumiError{
				command:      "remote-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-conn-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-cmd-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-mount-disk-dev-vdb",
				arch:         "arm64",
				vmCommand:    "mount-disk-dev-vdb",
				errorMessage: "error: Process exited with status 127: running \" nocommand /mnt/docker && mount /dev/vdb /mnt/docker\":\nbash: line 1: nocommand: command not found\n",
				vmName:       "ubuntu_22.04",
			},
		},
		{
			caseName: "SSHFailed",
			output:   outputSshFailed,
			result: pulumiError{
				command:      "remote-ci-630160752-4670-kernel-matrix-testing-system-probe-arm64-43724877-arm64-conn-arm64-fedora_37-no_usm-distro_arm64-ddvm-4-12288-cmd-arm64-fedora_37-no_usm-distro_arm64-ddvm-4-12288-mount-disk-dev-vdb",
				arch:         "arm64",
				vmCommand:    "mount-disk-dev-vdb",
				vmName:       "fedora_37",
				errorMessage: "error: proxy: after 60 failed attempts: ssh: rejected: connect failed (No route to host)\n",
			},
		},
		{
			caseName: "SSHFailedWithChangedOrder",
			output:   outputSshFailedWithChangedOrder,
			result: pulumiError{
				command:      "remote-ci-630160752-4670-kernel-matrix-testing-system-probe-arm64-43724877-arm64-conn-arm64-fedora_37-no_usm-distro_arm64-ddvm-4-12288-cmd-arm64-fedora_37-no_usm-distro_arm64-ddvm-4-12288-mount-disk-dev-vdb",
				arch:         "arm64",
				vmCommand:    "mount-disk-dev-vdb",
				vmName:       "fedora_37",
				errorMessage: "error: proxy: after 60 failed attempts: ssh: rejected: connect failed (No route to host)\n",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(tt *testing.T) {
			result := parsePulumiDiagnostics(c.output)
			require.NotNil(tt, result)
			require.Equal(tt, c.result, *result)
		})
	}
}

func TestParsePulumiCommand(t *testing.T) {
	cases := []struct {
		caseName string
		command  string
		arch     string
		vmCmd    string
		vmName   string
	}{
		{
			caseName: "NoVMSet",
			command:  "remote-gjulian-guillermo.julian-e2e-report-all-errors-ddvm-arm64-conn-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-cmd-arm64-ubuntu_22.04-distro_arm64-ddvm-4-8192-mount-disk-dev-vdb",
			arch:     "arm64",
			vmCmd:    "mount-disk-dev-vdb",
			vmName:   "ubuntu_22.04",
		},
		{
			caseName: "CommandWithoutVM",
			command:  "remote-aws-ci-634872953-4670-kernel-matrix-testing-system-probe-x86-64-44043832-x86_64-cmd-only_usm-distro_x86_64-download-with-curl",
			arch:     "x86_64",
			vmCmd:    "download-with-curl",
			vmName:   "",
		},
		{
			caseName: "DomainCreationCommand",
			command:  "remote-aws-ci-632806887-4670-kernel-matrix-testing-system-probe-arm64-43913143-arm64-cmd-arm64-debian_12-distro_arm64-no_usm-ddvm-4-12288-create-nvram",
			arch:     "arm64",
			vmCmd:    "create-nvram",
			vmName:   "debian_12",
		},
		{
			caseName: "AlteredTagOrder",
			command:  "remote-ci-632806887-4670-kernel-matrix-testing-system-probe-arm64-43913143-arm64-conn-arm64-ubuntu_23.10-only_usm-distro_arm64-ddvm-4-12288-cmd-arm64-ubuntu_23.10-only_usm-distro_arm64-ddvm-4-12288-set-docker-data-root",
			arch:     "arm64",
			vmCmd:    "set-docker-data-root",
			vmName:   "ubuntu_23.10",
		},
		{
			caseName: "CommandWithVMSet",
			command:  "remote-ci-630160752-4670-kernel-matrix-testing-system-probe-arm64-43724877-arm64-conn-arm64-fedora_37-no_usm-distro_arm64-ddvm-4-12288-cmd-arm64-fedora_37-no_usm-distro_arm64-ddvm-4-12288-mount-disk-dev-vdb",
			arch:     "arm64",
			vmCmd:    "mount-disk-dev-vdb",
			vmName:   "fedora_37",
		},
	}

	for _, c := range cases {
		t.Run(c.caseName, func(tt *testing.T) {
			arch, vmCmd, vmName := parsePulumiComand(c.command)
			require.Equal(tt, c.arch, arch)
			require.Equal(tt, c.vmCmd, vmCmd)
			require.Equal(tt, c.vmName, vmName)
		})
	}
}
