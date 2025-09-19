package com_datadoghq_gitlab_commits

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	support "github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/bundle-support/test"
)

var testContext = context.TODO()

type GitlabCommitsTestSuite struct {
	support.MockHttpServerSuite
}

func TestGitlabCommits(t *testing.T) {
	suite.Run(t, new(GitlabCommitsTestSuite))
}
