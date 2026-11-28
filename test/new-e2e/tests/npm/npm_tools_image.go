// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// npmToolsImage is the name the suites run apps-npm-tools under on the agent host.
// pullNPMToolsImage stores the image under this name wherever it was pulled from, so the
// `docker run` and docker-compose calls of the tests find it locally and never pull it.
const npmToolsImage = "ghcr.io/datadog/apps-npm-tools:" + apps.Version

// pullNPMToolsImage pulls image on host and stores it as npmToolsImage. image is the
// apps-npm-tools image the suite's provisioner resolved with apps.Image, like the framework
// components do: the internal registry hosting the workload apps (agent-qa ECR on AWS), or
// ghcr.io when the environment has none. Pulls from ghcr.io time out often enough to fail whole
// suites in SetupSuite, so a failed internal pull fails the suite rather than falling back to it.
func pullNPMToolsImage(host *components.RemoteHost, image string) {
	if image == npmToolsImage {
		host.MustExecute("docker pull " + npmToolsImage)
		return
	}
	// The suites only run on AWS. A private docker config directory enables the ECR credential
	// helper without touching the user's config, and is removed whether or not the pull succeeds.
	host.MustExecute(fmt.Sprintf(
		`cfg=$(mktemp -d) && trap 'rm -rf "$cfg"' EXIT && echo '{"credsStore":"ecr-login"}' > "$cfg/config.json" && DOCKER_CONFIG="$cfg" docker pull %[1]s && docker tag %[1]s %[2]s`,
		image, npmToolsImage))
}
