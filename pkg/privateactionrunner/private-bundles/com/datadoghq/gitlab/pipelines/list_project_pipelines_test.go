package com_datadoghq_gitlab_pipelines

import (
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
)

func (suite *GitlabPipelinesTestSuite) TestListProjectPipelines() {
	handler := NewListProjectPipelinesHandler()
	inputs := map[string]any{"project_id": "1234"}

	// When the API response is correct
	suite.Run("success", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1234/pipelines", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodGet)
			_, _ = fmt.Fprint(w, `[{"id":1},{"id":2}]`)
		})
		expected := &ListProjectPipelinesOutputs{Pipelines: []*gitlab.PipelineInfo{{ID: 1}, {ID: 2}}}
		got, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.NoError(err)
		suite.Equal(expected, got)
	})

	// When the API failed to return a response
	suite.Run("fail", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1234/pipelines", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodGet)
			http.Error(w, "project 1234 not found", http.StatusNotFound)
		})
		_, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.Error(err)
		suite.Contains(err.Error(), "could not list project pipelines")
	})
}
