// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package docker

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// InstallDocker installs Docker on the host at provision time.
// Call this explicitly for cloud environments that do not use pre-baked AMIs
// (Azure, GCP). AWS e2e AMIs have Docker pre-baked.
func InstallDocker(host *remoteComp.Host, opts ...pulumi.ResourceOption) (command.Command, error) {
	return host.OS.PackageManager().Ensure("docker", nil, "docker", os.WithPulumiResourceOptions(opts...))
}

// InstallCompose installs docker-compose on the host at provision time.
// Call this explicitly for cloud environments that do not use pre-baked AMIs
// (Azure, GCP). AWS e2e AMIs have docker-compose v2.27.0 pre-baked.
func InstallCompose(host *remoteComp.Host, opts ...pulumi.ResourceOption) (command.Command, error) {
	installCompose := pulumi.Sprintf(
		"curl --retry 10 -fsSLo /usr/local/bin/docker-compose https://github.com/docker/compose/releases/download/%s/docker-compose-linux-$(uname -p) && sudo chmod 755 /usr/local/bin/docker-compose",
		composeVersion,
	)
	return host.OS.Runner().Command("install-compose", &command.Args{Create: installCompose, Sudo: true}, opts...)
}
