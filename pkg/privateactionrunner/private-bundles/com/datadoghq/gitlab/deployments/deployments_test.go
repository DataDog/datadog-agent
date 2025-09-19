package com_datadoghq_gitlab_deployments

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	support "github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/bundle-support/test"
)

var testContext = context.TODO()

type GitlabDeploymentsTestSuite struct {
	support.MockHttpServerSuite
}

func TestGitlabDeployments(t *testing.T) {
	suite.Run(t, new(GitlabDeploymentsTestSuite))
}
