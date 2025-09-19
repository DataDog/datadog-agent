package com_datadoghq_gitlab_repository_files

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	support "github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/bundle-support/test"
)

var testContext = context.TODO()

type GitlabRepositoryFilesTestSuite struct {
	support.MockHttpServerSuite
}

func TestGitlabDeployments(t *testing.T) {
	suite.Run(t, new(GitlabRepositoryFilesTestSuite))
}
