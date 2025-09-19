package com_datadoghq_gitlab_repository_files

import (
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/dd-source/domains/actionplatform/apps/private-runner/src/private-bundles/com/datadoghq/gitlab/lib"
)

func (suite *GitlabRepositoryFilesTestSuite) TestCreateProjectForUser() {
	handler := NewGetRawFileHandler()
	inputs := map[string]any{"project_id": 1, "file_path": "lib/class.rb"}

	// When the API response is correct
	suite.Run("utf8 success", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1/repository/files/lib%2Fclass%2Erb/raw", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodGet)
			_, _ = fmt.Fprint(w, `# Ruby file`)
		})
		expected := &GetRawFileOutputs{Content: `# Ruby file`}
		got, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.NoError(err)
		suite.Equal(expected, got)
	})

	// When the API failed to return a response
	suite.Run("fail", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1/repository/files/lib%2Fclass%2Erb/raw", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodGet)
			http.Error(w, "file lib/class.rb not found", http.StatusNotFound)
		})
		_, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.Error(err)
		suite.Equal(gitlab.ErrNotFound, err)
	})

	// When the encoding is base64
	inputs = map[string]any{"project_id": 1, "file_path": "lib/class.rb", "encoding": "base64"}
	suite.Run("base64 success", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1/repository/files/lib%2Fclass%2Erb/raw", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodGet)
			_, _ = fmt.Fprint(w, `# Ruby file`)
		})
		expected := &GetRawFileOutputs{Content: []byte(`# Ruby file`)}
		got, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.NoError(err)
		suite.Equal(expected, got)
	})

	// When a ref is provided
	inputs = map[string]any{"project_id": 1, "file_path": "lib/class.rb", "ref": "main"}
	suite.Run("success with ref", func() {
		mux, server := suite.SetupServer(false)
		mux.HandleFunc("/api/v4/projects/1/repository/files/lib%2Fclass%2Erb/raw", func(w http.ResponseWriter, r *http.Request) {
			suite.AssertMethod(r, http.MethodGet)
			suite.AssertQuery(r, "ref", "main")
			_, _ = fmt.Fprint(w, `# Ruby file`)
		})
		expected := &GetRawFileOutputs{Content: `# Ruby file`}
		got, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.NoError(err)
		suite.Equal(expected, got)
	})

	// When the encoding is invalid
	inputs = map[string]any{"project_id": 1, "file_path": "lib/class.rb", "encoding": "foo"}
	suite.Run("encoding failure", func() {
		_, server := suite.SetupServer(false)
		_, err := handler.Run(testContext, lib.NewTestTask(inputs), lib.NewTestCredential(server.URL))
		suite.Error(err)
	})
}
