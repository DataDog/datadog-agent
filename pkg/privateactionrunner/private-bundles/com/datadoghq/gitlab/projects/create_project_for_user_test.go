package com_datadoghq_gitlab_projects

import (
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
)

func (suite *GitlabProjectsTestSuite) TestCreateProjectForUser() {
	handler := NewCreateProjectForUserHandler()
	inputs := map[string]any{"user_id": 1, "name": "Test project", "path": "test-project"}

	// When the API response is correct
	suite.Run("success", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/user/1", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodPost)
			_, _ = fmt.Fprint(w, `{"id":1}`)
		})
		expected := &CreateProjectForUserOutputs{Project: &gitlab.Project{ID: 1}}
		got, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.NoError(err)
		suite.Equal(expected, got)
	})

	// When the API failed to return a response
	suite.Run("fail", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/user/1", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodPost)
			http.Error(w, "project name already taken", http.StatusNotFound)
		})
		_, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.Error(err)
		suite.Equal(gitlab.ErrNotFound, err)
	})

	// When the user provided optional fields
	inputs = map[string]any{"user_id": 1, "name": "Test project", "path": "test-project", "allow_merge_on_skipped_pipeline": true}
	suite.Run("success for optional fields", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/user/1", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodPost)
			_, _ = fmt.Fprint(w, `{"id":1}`)
		})
		expected := &CreateProjectForUserOutputs{Project: &gitlab.Project{ID: 1}}
		got, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.NoError(err)
		suite.Equal(expected, got)
	})
}
