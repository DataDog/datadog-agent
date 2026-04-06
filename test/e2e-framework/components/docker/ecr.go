// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package docker

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// InstallECRCredentialsHelper installs the Amazon ECR credential helper and jq on the host,
// then merges credsStore=ecr-login into ~/.docker/config.json (preserving existing keys).
// This enables automatic authentication against ECR registries (including pull-through caches).
func InstallECRCredentialsHelper(n namer.Namer, host *remoteComp.Host, opts ...pulumi.ResourceOption) (command.Command, error) {
	ecrCredsHelperInstall, err := host.OS.PackageManager().Ensure("amazon-ecr-credential-helper", nil, "docker-credential-ecr-login", os.WithPulumiResourceOptions(opts...))
	if err != nil {
		return nil, err
	}

	jqInstall, err := host.OS.PackageManager().Ensure("jq", nil, "jq", os.WithPulumiResourceOptions(opts...))
	if err != nil {
		return nil, err
	}

	// Merge credsStore into existing ~/.docker/config.json so we do not wipe auths, credHelpers, proxies, etc.
	mergeDockerConfig := `mkdir -p "${HOME}/.docker" && ` +
		`([ -s "${HOME}/.docker/config.json" ] || echo '{}' > "${HOME}/.docker/config.json") && ` +
		`TMP=$(mktemp) && jq '. + {"credsStore": "ecr-login"}' "${HOME}/.docker/config.json" > "$TMP" && ` +
		`mv "$TMP" "${HOME}/.docker/config.json"`

	ecrConfigCommand, err := host.OS.Runner().Command(
		n.ResourceName("ecr-config"),
		&command.Args{
			Create: pulumi.String(mergeDockerConfig),
			Sudo:   false,
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(ecrCredsHelperInstall, jqInstall))...,
	)
	if err != nil {
		return nil, err
	}

	return ecrConfigCommand, nil
}
