// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package docker

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// SetupECRDockerAuth merges credsStore=ecr-login into ~/.docker/config.json (preserving existing
// keys). docker-credential-ecr-login and jq must already be present on the host (pre-baked in
// AWS e2e AMIs). This enables automatic authentication against ECR registries (including
// pull-through caches).
func SetupECRDockerAuth(n namer.Namer, host *remoteComp.Host, opts ...pulumi.ResourceOption) (command.Command, error) {
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
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return ecrConfigCommand, nil
}
