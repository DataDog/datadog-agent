package operator

import (
	"fmt"

	"github.com/Masterminds/semver"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
)

const (
	defaultOperatorImageRepo = "gcr.io/datadoghq/operator"
	defaultOperatorImageTag  = "latest"
)

func dockerOperatorFullImagePath(e config.Env, repositoryPath, imageTag string) string {
	// return operator image path if defined
	if e.OperatorFullImagePath() != "" {
		return e.OperatorFullImagePath()
	}

	// if operator pipeline id and commit sha are defined, use the image from the pipeline pushed on agent QA registry
	if e.PipelineID() != "" && e.CommitSHA() != "" {
		return utils.BuildDockerImagePath(fmt.Sprintf("%s/operator", e.InternalRegistry()), fmt.Sprintf("%s-%s", e.PipelineID(), e.CommitSHA()))
	}

	if repositoryPath == "" {
		repositoryPath = defaultOperatorImageRepo
	}
	if imageTag == "" {
		imageTag = dockerOperatorImageTag(e, config.OperatorSemverVersion)
	}

	return utils.BuildDockerImagePath(repositoryPath, imageTag)
}

func dockerOperatorImageTag(e config.Env, semverVersion func(config.Env) (*semver.Version, error)) string {
	// default tag
	operatorImageTag := defaultOperatorImageTag

	// try parse operator version
	operatorVersion, err := semverVersion(e)
	if operatorVersion != nil && err == nil {
		operatorImageTag = operatorVersion.String()
	} else {
		e.Ctx().Log.Debug("Unable to parse operator version, using latest", nil)
	}

	return operatorImageTag
}
