// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestCreateArchive(t *testing.T) {
	common.SetupConfig("./test")
	mockConfig := config.NewMock()
	mockConfig.Set("confd_path", "./test/confd")
	mockConfig.Set("log_file", "./test/logs/agent.log")
	zipFilePath := getArchivePath()
	filePath, err := createArchive(zipFilePath, true, SearchPaths{}, "")

	assert.Nil(t, err)
	assert.Equal(t, zipFilePath, filePath)

	if _, err := os.Stat(zipFilePath); os.IsNotExist(err) {
		assert.Fail(t, "The Zip File was not created")
	} else {
		os.Remove(zipFilePath)
	}
}

// The zipfile should be created even if there is no config file.
func TestCreateArchiveBadConfig(t *testing.T) {
	common.SetupConfig("")
	zipFilePath := getArchivePath()
	filePath, err := createArchive(zipFilePath, true, SearchPaths{}, "")

	assert.Nil(t, err)
	assert.Equal(t, zipFilePath, filePath)

	if _, err := os.Stat(zipFilePath); os.IsNotExist(err) {
		assert.Fail(t, "The Zip File was not created")
	} else {
		os.Remove(zipFilePath)
	}
}

// Ensure sensitive data is redacted
func TestZipConfigCheck(t *testing.T) {
	cr := response.ConfigCheckResponse{
		Configs: make([]integration.Config, 0),
	}
	cr.Configs = append(cr.Configs, integration.Config{
		Name:      "TestCheck",
		Instances: []integration.Data{[]byte("username: User\npassword: MySecurePass")},
		Provider:  "FooProvider",
	})

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, _ := json.Marshal(cr)
		w.Write(out)
	}))
	defer ts.Close()
	ConfigCheckURL = ts.URL

	dir, err := ioutil.TempDir("", "TestZipConfigCheck")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	zipConfigCheck(dir, "")
	content, err := ioutil.ReadFile(filepath.Join(dir, "config-check.log"))
	if err != nil {
		log.Fatal(err)
	}

	assert.NotContains(t, string(content), "MySecurePass")
}
