// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dogstatsdstandalone

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
)

const (
	defaultDogstatsdImageRepo = "gcr.io/datadoghq/dogstatsd"
	defaultDogstatsdImageTag  = "latest"
)

func dockerDogstatsdFullImagePath(e config.Env, repositoryPath string) string {
	// return dogstatsd image path if defined
	if e.DogstatsdFullImagePath() != "" {
		return e.DogstatsdFullImagePath()
	}

	// if agent pipeline id and commit sha are defined, use the image from the pipeline pushed on agent QA registry
	if e.PipelineID() != "" && e.CommitSHA() != "" {
		return utils.BuildDockerImagePath(fmt.Sprintf("%s/dogstatsd-qa", e.InternalRegistry()), fmt.Sprintf("%s-%s", e.PipelineID(), e.CommitSHA()))
	}

	if repositoryPath == "" {
		repositoryPath = defaultDogstatsdImageRepo
	}

	return utils.BuildDockerImagePath(repositoryPath, defaultDogstatsdImageTag)
}
