package dogstatsdstandalone

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
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
		return utils.BuildDockerImagePath(fmt.Sprintf("%s/dogstatsd", e.InternalRegistry()), fmt.Sprintf("%s-%s", e.PipelineID(), e.CommitSHA()))
	}

	if repositoryPath == "" {
		repositoryPath = defaultDogstatsdImageRepo
	}

	return utils.BuildDockerImagePath(repositoryPath, defaultDogstatsdImageTag)
}
