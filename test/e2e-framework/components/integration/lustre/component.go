// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package lustre is the reusable host component for the Lustre 2.15.6 integration
// lab. It owns the bootstrap shell scripts (embedded here) and exposes command
// constructors that model the two host roles:
//
//   - Server (combined MGS+MDS+OSS on ldiskfs): ServerUserData. The server needs
//     the Whamcloud patched kernel 4.18.0-553.27.1.el8_lustre for the ldiskfs OSD
//     kmods, so it installs the kernel + a boot-time systemd oneshot (format/mount)
//     and reboots via cloud-init user-data on first boot — before the framework's
//     first SSH — so no Pulumi remote command is ever severed by the reboot.
//   - Client (Lustre client + Datadog Agent): NewClientSetup mounts the filesystem
//     over LNet tcp0 and grants dd-agent passwordless sudo to the Lustre CLIs, and
//     NewLoad drives continuous fio I/O so the check's performance families are non-zero.
//
// The scenario (scenarios/aws/integrations/lustre) imports this component and wires
// the constructors into its Pulumi program; the check conf.yaml stays scenario-owned.
package lustre

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Default topology values for the Lustre lab. Callers may override them.
const (
	// DefaultFilesystemName is the Lustre filesystem name (fsname) created on the
	// server and mounted by the client.
	DefaultFilesystemName = "lustrefs"
	// DefaultLNetInterface is the network interface LNet is configured over.
	DefaultLNetInterface = "eth0"
	// DefaultClientMountpoint is where the client mounts the Lustre filesystem.
	DefaultClientMountpoint = "/mnt/lustre"
)

//go:embed scripts/server_userdata.sh
var serverUserDataScript string

//go:embed scripts/client_setup.sh
var clientSetupScript string

//go:embed scripts/lustre_load.sh
var loadScript string

// runRootScript runs a bootstrap script entirely as root on the host.
//
// The script must NOT be handed to the runner as a bare Create with Sudo:true:
// the framework builds `sudo <Create>`, and a script whose first line is a
// `#!/bin/bash` shebang would become `sudo #!/bin/bash ...` — the `#` comments
// out the rest of the line, `sudo` runs with no args, and the body then runs
// UNPRIVILEGED. Instead we run `sudo bash -s` (Create="bash -s" + Sudo:true) and
// feed the script on stdin, where the shebang is just an inert first comment
// line. Environment values are prepended as assignment lines inside the stdin
// payload so they reach the root shell regardless of sudo's env_reset policy
// (sudo -E / the Environment map would not survive env_reset).
func runRootScript(e config.Env, host *remote.Host, name string, stdin pulumi.StringInput, opts ...pulumi.ResourceOption) (command.Command, error) {
	return host.OS.Runner().Command(
		e.CommonNamer().ResourceName(name),
		&command.Args{
			Create: pulumi.String("bash -s"),
			Sudo:   true,
			Stdin:  stdin,
		},
		opts...,
	)
}

// ServerUserData returns the EC2 cloud-init user-data that provisions the Lustre
// server on first boot: it installs the Whamcloud patched kernel + ldiskfs OSD
// kmods and a boot-time systemd oneshot that formats + mounts the targets, then
// reboots into the patched kernel — all before the framework's first SSH, so no
// Pulumi remote command is ever severed by the reboot. The client waits for the
// server (lctl ping + retry mount) from its own host, which never reboots.
func ServerUserData(fsname, iface string) string {
	return fmt.Sprintf(
		"#!/bin/bash\nLUSTRE_FSNAME=%q\nLUSTRE_LNET_IFACE=%q\n%s",
		fsname, iface, serverUserDataScript,
	)
}

// NewClientSetup installs the Lustre client kmod (no reboot), configures LNet,
// and mounts the server filesystem at mountpoint. serverNID is the server's LNet
// NID (e.g. "<ip>@tcp", typically a resolved Pulumi output). clientVersion selects
// the Whamcloud Lustre release (e.g. "2.15.6" or "2.16.1") and elPath selects the
// matching client repo path (e.g. "el8.10" or "el9.4"); together they form the
// repo baseurl that client_setup.sh installs from. The 2.16.x client RPMs exist
// only for el9.4, so a 2.16.1 client must be paired with an EL9.4 elPath/AMI.
func NewClientSetup(e config.Env, host *remote.Host, fsname, iface, mountpoint, clientVersion, elPath string, serverNID pulumi.StringInput, opts ...pulumi.ResourceOption) (command.Command, error) {
	stdin := pulumi.Sprintf(
		"LUSTRE_FSNAME=%q\nLUSTRE_LNET_IFACE=%q\nLUSTRE_MOUNTPOINT=%q\nLUSTRE_CLIENT_VERSION=%q\nLUSTRE_CLIENT_EL_PATH=%q\nLUSTRE_SERVER_NID=\"%s\"\n%s",
		fsname, iface, mountpoint, clientVersion, elPath, serverNID, clientSetupScript,
	)
	return runRootScript(e, host, "lustre-client-setup", stdin, opts...)
}

// NewLoad installs and starts a continuous I/O generator (systemd service) on the
// client that reads/writes the mounted Lustre filesystem, so the check's
// performance metric families (llite/osc/mdc bandwidth + IOPS, LNET traffic) are
// non-zero rather than only config/capacity coverage.
func NewLoad(e config.Env, host *remote.Host, mountpoint string, opts ...pulumi.ResourceOption) (command.Command, error) {
	stdin := pulumi.String(fmt.Sprintf(
		"LUSTRE_MOUNTPOINT=%q\n%s",
		mountpoint, loadScript,
	))
	return runRootScript(e, host, "lustre-load", stdin, opts...)
}
