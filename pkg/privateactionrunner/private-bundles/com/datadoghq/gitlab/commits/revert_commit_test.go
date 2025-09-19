package com_datadoghq_gitlab_commits

import (
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
)

func (suite *GitlabCommitsTestSuite) TestRevertCommit() {
	handler := NewRevertCommitHandler()
	inputs := map[string]any{"project_id": "1234", "sha": "a738f717824ff53aebad8b090c1b79a14f2bd9e8"}

	// When the API response is correct
	suite.Run("success", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1234/repository/commits/a738f717824ff53aebad8b090c1b79a14f2bd9e8/revert", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodPost)
			_, _ = fmt.Fprint(w, `{"id":"a738f7"}`)
		})
		expected := &RevertCommitOutputs{Commit: &gitlab.Commit{ID: "a738f7"}}
		got, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.NoError(err)
		suite.Equal(expected, got)
	})

	// When the API failed to return a response
	suite.Run("fail", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1234/repository/commits/a738f717824ff53aebad8b090c1b79a14f2bd9e8/revert", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodPost)
			http.Error(w, "project 1234 not found", http.StatusNotFound)
		})
		_, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.Error(err)
		suite.Equal(gitlab.ErrNotFound, err)
	})
}
