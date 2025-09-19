package com_datadoghq_gitlab_pipelines

import (
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
)

func (suite *GitlabPipelinesTestSuite) TestCreatePipeline() {
	handler := NewCreatePipelineHandler()
	inputs := map[string]any{"project_id": "1234", "ref": "a", "variables": []*gitlab.PipelineVariableOptions{{}}}

	// When the API response is correct
	suite.Run("success", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1234/pipeline", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodPost)
			_, _ = fmt.Fprint(w, `{"id":1}`)
		})
		expected := &CreatePipelineOutputs{Pipeline: &gitlab.Pipeline{ID: 1}}
		got, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.NoError(err)
		suite.Equal(expected, got)
	})

	// When the API failed to return a response
	suite.Run("fail", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1234/pipeline", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodPost)
			http.Error(w, "project 1234 not found", http.StatusNotFound)
		})
		_, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.Error(err)
		suite.Equal(gitlab.ErrNotFound, err)
	})
}
