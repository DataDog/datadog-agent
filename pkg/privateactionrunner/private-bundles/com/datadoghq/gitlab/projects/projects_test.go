package com_datadoghq_gitlab_projects

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	support "github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/bundle-support/test"
)

var testContext = context.TODO()

type GitlabProjectsTestSuite struct {
	support.MockHttpServerSuite
}

func TestGitlabProjects(t *testing.T) {
	suite.Run(t, new(GitlabProjectsTestSuite))
}
