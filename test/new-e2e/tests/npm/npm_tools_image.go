// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// npmToolsImage is the name the suites run apps-npm-tools under on the agent host.
// pullNPMToolsImage stores the image under this name wherever it was pulled from, so the
// `docker run` and docker-compose calls of the tests find it locally and never pull it.
const npmToolsImage = "ghcr.io/datadog/apps-npm-tools:" + apps.Version

// internalNPMToolsImage returns the apps-npm-tools image in the internal registry, or an empty
// string when no registry is configured. The workload app images are published at the root of the
// first registry of the image pull registry parameter, which may hold a comma-separated list.
func internalNPMToolsImage(imagePullRegistry string) string {
	reg := strings.SplitN(imagePullRegistry, ",", 2)[0]
	if reg == "" {
		return ""
	}
	return reg + "/apps-npm-tools:" + apps.Version
}

// pullNPMToolsImage pulls apps-npm-tools on host and stores it as npmToolsImage. Pulls from
// ghcr.io time out often enough to fail whole suites in SetupSuite, so when an internal registry is
// configured, as in CI, the image comes from there and a failed pull fails the suite. ghcr.io is only
// used for runs without a registry.
func pullNPMToolsImage(host *components.RemoteHost) {
	reg, _ := runner.GetProfile().ParamStore().GetWithDefault(parameters.ImagePullRegistry, "")
	image := internalNPMToolsImage(reg)
	if image == "" {
		host.MustExecute("docker pull " + npmToolsImage)
		return
	}
	// A private docker config directory enables the ECR credential helper without touching the
	// user's config, and is removed whether or not the pull succeeds.
	host.MustExecute(fmt.Sprintf(
		`cfg=$(mktemp -d) && trap 'rm -rf "$cfg"' EXIT && echo '{"credsStore":"ecr-login"}' > "$cfg/config.json" && DOCKER_CONFIG="$cfg" docker pull %[1]s && docker tag %[1]s %[2]s`,
		image, npmToolsImage))
}
