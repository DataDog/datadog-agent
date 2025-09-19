package com_datadoghq_gitlab_repositories

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	support "github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/bundle-support/test"
)

var testContext = context.TODO()

type GitlabRepositoriesTestSuite struct {
	support.MockHttpServerSuite
}

func TestGitlabRepositories(t *testing.T) {
	suite.Run(t, new(GitlabRepositoriesTestSuite))
}
