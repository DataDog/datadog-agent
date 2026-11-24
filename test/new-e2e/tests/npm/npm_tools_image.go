// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// npmToolsImage is the name the suites run apps-npm-tools under on the agent host.
// pullNPMToolsImage stores the image under this name wherever it was pulled from, so the
// `docker run` and docker-compose calls of the tests find it locally and never pull it.
const npmToolsImage = "ghcr.io/datadog/apps-npm-tools:" + apps.Version

// npmToolsDockerConfig is the docker config directory used for the internal registry pull. A
// dedicated directory enables the ECR credential helper without touching the user's config.
const npmToolsDockerConfig = "/tmp/npm-tools-docker"

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

// pullNPMToolsImage pulls apps-npm-tools on host from the internal registry and tags it as
// npmToolsImage. Pulls from ghcr.io time out often enough to fail whole suites in SetupSuite, so
// ghcr.io is only the fallback for a host that cannot pull from the internal registry.
func pullNPMToolsImage(t testing.TB, host *components.RemoteHost) {
	reg, _ := runner.GetProfile().ParamStore().GetWithDefault(parameters.ImagePullRegistry, "")
	if image := internalNPMToolsImage(reg); image != "" {
		_, err := host.Execute(fmt.Sprintf(
			`mkdir -p %[1]s && echo '{"credsStore":"ecr-login"}' > %[1]s/config.json && DOCKER_CONFIG=%[1]s docker pull %[2]s && docker tag %[2]s %[3]s`,
			npmToolsDockerConfig, image, npmToolsImage))
		if err == nil {
			return
		}
		t.Logf("failed to pull %s, falling back to %s: %v", image, npmToolsImage, err)
	}
	host.MustExecute("docker pull " + npmToolsImage)
}
