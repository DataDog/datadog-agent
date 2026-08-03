// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package docker

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ecrCredentialHelperVersion pins the docker-credential-ecr-login release
// installed on Red Hat family distros. Bump it manually (or via Renovate):
// resolving the "latest" release from GitHub at provision time was a source of
// flakiness. Keep it in sync with the pin in
// test/new-e2e/tests/installer/host/host.go.
const ecrCredentialHelperVersion = "0.12.0"

// SetupECRDockerAuth merges credsStore=ecr-login into ~/.docker/config.json (preserving existing
// keys). docker-credential-ecr-login and jq are expected to already be present on the host
// (pre-baked in AWS e2e AMIs); this enables automatic authentication against ECR registries
// (including pull-through caches).
//
// TODO(ACIX-1305 follow-up): RHEL family has no -e2e AMI yet (introduced by the SBOM/RHEL10
// work in #51486), so the helper binary and jq are still installed at runtime there. Migrated
// OSes assume both are pre-baked. Remove the RHEL-family install once a RHEL 10 -e2e AMI bakes
// them.
func SetupECRDockerAuth(n namer.Namer, host *remoteComp.Host, opts ...pulumi.ResourceOption) (command.Command, error) {
	switch host.OS.Descriptor().Flavor {
	case os.RedHat, os.CentOS, os.RockyLinux, os.AlmaLinux, os.AmazonLinux:
		ecrCredsHelperInstall, err := ensureECRCredentialHelper(n, host, opts...)
		if err != nil {
			return nil, err
		}

		jqInstall, err := host.OS.PackageManager().Ensure("jq", nil, "jq", os.WithPulumiResourceOptions(opts...))
		if err != nil {
			return nil, err
		}

		opts = utils.MergeOptions(opts, utils.PulumiDependsOn(ecrCredsHelperInstall, jqInstall))
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
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return ecrConfigCommand, nil
}

// ensureECRCredentialHelper installs the docker-credential-ecr-login binary.
// Red Hat family distributions have no package for it, so the pinned static
// release binary is fetched directly; other distributions install it through
// their package manager.
func ensureECRCredentialHelper(n namer.Namer, host *remoteComp.Host, opts ...pulumi.ResourceOption) (command.Command, error) {
	switch host.OS.Descriptor().Flavor {
	case os.RedHat, os.CentOS, os.RockyLinux, os.AlmaLinux:
		// sudo cannot run a bare "if" compound, so feed the script to bash on
		// stdin (sudo bash <<EOF), matching the kubeadm provisioner's rootScript.
		install := fmt.Sprintf(`bash <<'EOF'
set -euo pipefail
if ! command -v docker-credential-ecr-login; then
  a=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
  curl -fsSL -o /usr/local/bin/docker-credential-ecr-login "https://amazon-ecr-credential-helper-releases.s3.us-east-2.amazonaws.com/%s/linux-${a}/docker-credential-ecr-login"
  chmod 0755 /usr/local/bin/docker-credential-ecr-login
fi
EOF`, ecrCredentialHelperVersion)
		return host.OS.Runner().Command(
			n.ResourceName("ecr-credential-helper"),
			&command.Args{Create: pulumi.String(install), Sudo: true},
			opts...,
		)
	default:
		return host.OS.PackageManager().Ensure("amazon-ecr-credential-helper", nil, "docker-credential-ecr-login", os.WithPulumiResourceOptions(opts...))
	}
}
