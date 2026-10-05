// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lustre

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	lustrecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/integration/lustre"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// The reusable Lustre bootstrap (embedded scripts + command constructors that
// model the server and client host roles) lives in the shared component package
// components/integration/lustre. These thin wrappers pin this scenario's topology
// constants and delegate to it, so run.go reads as a plain provisioning sequence.
//
// The server needs the patched kernel 4.18.0-553.27.1.el8_lustre for the ldiskfs
// OSD kmods, so it installs the kernel + reboots via cloud-init user-data before
// the framework's first SSH (serverUserData); the client uses the stock EL8.10
// kernel with no reboot and waits for the server from its own host.

const (
	// filesystemName is the Lustre filesystem name (fsname) created on the server
	// and mounted by the client.
	filesystemName = lustrecomp.DefaultFilesystemName
	// lnetInterface is the network interface LNet is configured over.
	lnetInterface = lustrecomp.DefaultLNetInterface
	// clientMountpoint is where the client mounts the Lustre filesystem.
	clientMountpoint = lustrecomp.DefaultClientMountpoint
)

// serverUserData returns the EC2 cloud-init user-data that provisions the Lustre
// server (patched kernel + boot-time format/mount + reboot) on first boot, before
// the framework's first SSH. See lustrecomp.ServerUserData.
func serverUserData() string {
	return lustrecomp.ServerUserData(filesystemName, lnetInterface)
}

// newClientSetup installs the client kmod, configures LNet, and mounts the server
// filesystem. serverIP is the server's private IP (resolved Pulumi output); the
// NID is built as <ip>@tcp. clientVersion is the Whamcloud Lustre release for the
// client (e.g. "2.15.6" or "2.16.1") and elPath is the matching repo path (e.g.
// "el8.10" or "el9.4"). See lustrecomp.NewClientSetup.
func newClientSetup(e config.Env, host *remote.Host, serverIP pulumi.StringInput, clientVersion, elPath string, opts ...pulumi.ResourceOption) (command.Command, error) {
	serverNID := pulumi.Sprintf("%s@tcp", serverIP)
	return lustrecomp.NewClientSetup(e, host, filesystemName, lnetInterface, clientMountpoint, clientVersion, elPath, serverNID, opts...)
}

// newLoad starts the continuous I/O generator so the check's performance families
// are non-zero. See lustrecomp.NewLoad.
func newLoad(e config.Env, host *remote.Host, opts ...pulumi.ResourceOption) (command.Command, error) {
	return lustrecomp.NewLoad(e, host, clientMountpoint, opts...)
}
