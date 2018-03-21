// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/stretchr/testify/assert"
)

func TestMkURL(t *testing.T) {
	common.SetupConfig("./test")
	config.Datadog.Set("dd_url", "https://example.com")
	config.Datadog.Set("api_key", "123456")
	assert.Equal(t, "https://example.com/support/flare/999?api_key=123456", mkURL("999"))
	assert.Equal(t, "https://example.com/support/flare?api_key=123456", mkURL(""))
}

func TestFlareHasRightForm(t *testing.T) {
	var lastRequest *http.Request

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lastRequest = r
		err := lastRequest.ParseMultipartForm(1000000)
		assert.Nil(t, err)
		io.WriteString(w, "{}")
	}))
	defer ts.Close()

	ddURL := ts.URL

	config.Datadog.Set("dd_url", ddURL)

	archivePath := "./test/blank.zip"
	caseID := "12345"
	email := "dev@datadoghq.com"

	_, err := SendFlare(archivePath, caseID, email)

	assert.Nil(t, err)

	av, _ := version.New(version.AgentVersion, version.Commit)

	assert.Equal(t, caseID, lastRequest.FormValue("case_id"))
	assert.Equal(t, email, lastRequest.FormValue("email"))
	assert.Equal(t, av.String(), lastRequest.FormValue("agent_version"))

}
