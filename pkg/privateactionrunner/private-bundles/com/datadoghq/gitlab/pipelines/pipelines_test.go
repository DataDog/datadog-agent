package com_datadoghq_gitlab_pipelines

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	support "github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/bundle-support/test"
)

var testContext = context.TODO()

type GitlabPipelinesTestSuite struct {
	support.MockHttpServerSuite
}

func TestGitlabPipelines(t *testing.T) {
	suite.Run(t, new(GitlabPipelinesTestSuite))
}
