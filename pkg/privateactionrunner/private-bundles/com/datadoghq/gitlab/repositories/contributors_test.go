package com_datadoghq_gitlab_repositories

import (
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
)

func (suite *GitlabRepositoriesTestSuite) TestCreateProject() {
	handler := NewContributorsHandler()
	inputs := map[string]any{"project_id": 1}

	// When the API response is correct
	suite.Run("success", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1/repository/contributors", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodGet)
			_, _ = fmt.Fprint(w, `[{"name":"John"}]`)
		})
		expected := &ContributorsOutputs{Contributors: []*gitlab.Contributor{{Name: "John"}}}
		got, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.NoError(err)
		suite.Equal(expected, got)
	})

	// When the API failed to return a response
	suite.Run("fail", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1/repository/contributors", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodGet)
			http.Error(w, "project 1 not found", http.StatusNotFound)
		})
		_, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.Error(err)
		suite.Equal(gitlab.ErrNotFound, err)
	})

	// When the user provided optional fields
	inputs = map[string]any{"project_id": 1, "ref": "main", "sort": "name"}
	suite.Run("success for optional fields", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1/repository/contributors", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodGet)
			suite.AssertQuery(r, "ref", "main")
			suite.AssertQuery(r, "sort", "name")
			_, _ = fmt.Fprint(w, `[{"name":"John"}]`)
		})
		expected := &ContributorsOutputs{Contributors: []*gitlab.Contributor{{Name: "John"}}}
		got, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.NoError(err)
		suite.Equal(expected, got)
	})
}
