// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package flare

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestCreateArchive(t *testing.T) {
	common.SetupConfig("./test")
	mockConfig := config.Mock()
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

func TestCreateArchiveAndGoRoutines(t *testing.T) {

	contents := "No Goroutines for you, my friend!"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s", contents)
	}))
	defer ts.Close()

	pprofURL = ts.URL

	zipFilePath := getArchivePath()
	filePath, err := createArchive(zipFilePath, true, SearchPaths{}, "")

	assert.Nil(t, err)
	assert.Equal(t, zipFilePath, filePath)

	// Open a zip archive for reading.
	z, err := zip.OpenReader(zipFilePath)
	if err != nil {
		assert.Fail(t, "Unable to open the flare archive")
	}
	defer z.Close()
	defer os.Remove(zipFilePath)

	// Iterate through the files in the archive,
	// printing some of their contents.
	found := false
	for _, f := range z.File {

		// find go-routine dump.
		if path.Base(f.Name) == routineDumpFilename {
			found = true

			dump, err := f.Open()
			if err != nil {
				assert.Fail(t, "Unable to open go-routine dump")
			}
			defer dump.Close()

			routines, err := ioutil.ReadAll(dump)
			if err != nil {
				assert.Fail(t, "Unable to read go-routine dump")
			}

			assert.Equal(t, contents, string(routines[:]))
		}
	}

	assert.True(t, found, "Go routine dump not found in flare")
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
	configCheckURL = ts.URL

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

func TestIncludeSystemProbeConfig(t *testing.T) {
	assert := assert.New(t)
	common.SetupConfig("./test/datadog-agent.yaml")
	// create system-probe.yaml file because it's in .gitignore
	_, err := os.Create("./test/system-probe.yaml")
	assert.NoError(err, "couldn't create system-probe.yaml")
	defer os.Remove("./test/system-probe.yaml")

	zipFilePath := getArchivePath()
	filePath, err := createArchive(zipFilePath, true, SearchPaths{"": "./test/confd"}, "")
	assert.NoError(err)
	assert.Equal(zipFilePath, filePath)

	defer os.Remove(zipFilePath)

	z, err := zip.OpenReader(zipFilePath)
	assert.NoError(err, "opening the zip shouldn't pop an error")

	var hasDDConfig, hasSysProbeConfig bool
	for _, f := range z.File {
		if strings.HasSuffix(f.Name, "datadog-agent.yaml") {
			hasDDConfig = true
		}
		if strings.HasSuffix(f.Name, "system-probe.yaml") {
			hasSysProbeConfig = true
		}
	}

	assert.True(hasDDConfig, "datadog-agent.yaml should've been included")
	assert.True(hasSysProbeConfig, "system-probe.yaml should've been included")
}

func TestIncludeConfigFiles(t *testing.T) {
	assert := assert.New(t)

	common.SetupConfig("./test")
	zipFilePath := getArchivePath()
	filePath, err := createArchive(zipFilePath, true, SearchPaths{"": "./test/confd"}, "")

	assert.NoError(err)
	assert.Equal(zipFilePath, filePath)

	if _, err := os.Stat(zipFilePath); os.IsNotExist(err) {
		assert.Fail("The Zip File was not created")
	}

	defer os.Remove(zipFilePath)

	// asserts that test.yaml and test.yml have been included
	z, err := zip.OpenReader(zipFilePath)
	assert.NoError(err, "opening the zip shouldn't pop an error")

	yaml, yml := false, false
	for _, f := range z.File {
		if strings.HasSuffix(f.Name, "test.yaml") {
			yaml = true
		} else if strings.HasSuffix(f.Name, "test.Yml") {
			yml = true
		} else if strings.HasSuffix(f.Name, "not_included.conf") {
			assert.Fail("not_included.conf should not been included into the flare")
		}
	}

	assert.True(yml, "test.yml should've been included")
	assert.True(yaml, "test.yaml should've been included")
}

func TestCleanDirectoryName(t *testing.T) {
	insaneHostname := `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
	<html xmlns="http://www.w3.org/1999/xhtml">
	<head>
	<meta http-equiv="Content-Type" content="text/html; charset=iso-8859-1"/>
	<title>404 - File or directory not found.</title>
	<style type="text/css">
	<!--
	body{margin:0;font-size:.7em;font-family:Verdana, Arial, Helvetica, sans-serif;background:#EEEEEE;}
	fieldset{padding:0 15px 10px 15px;}
	h1{font-size:2.4em;margin:0;color:#FFF;}
	h2{font-size:1.7em;margin:0;color:#CC0000;}
	h3{font-size:1.2em;margin:10px 0 0 0;color:#000000;}
	background-color:#555555;}
	.content-container{background:#FFF;width:96%;margin-top:8px;padding:10px;position:relative;}
	-->
	</style>
	</head>
	<body>
	<div id="header"><h1>Server Error</h1></div>
	<div id="content">
	<div class="content-container"><fieldset>
	<h2>404 - File or directory not found.</h2>
	<h3>The resource you are looking for might have been removed, had its name changed, or is temporarily unavailable.</h3>
	</fieldset></div>
	</div>
	</body>
	</html>`

	cleanedHostname := cleanDirectoryName(insaneHostname)

	assert.Len(t, cleanedHostname, directoryNameMaxSize)
	assert.True(t, !directoryNameFilter.MatchString(cleanedHostname))
}

func TestZipTaggerList(t *testing.T) {
	tagMap := make(map[string]response.TaggerListEntity)
	tagMap["random_entity_name"] = response.TaggerListEntity{
		Sources: []string{"docker_source_name"},
		Tags:    []string{"docker_image:custom-agent:latest", "image_name:custom-agent"},
	}
	resp := response.TaggerListResponse{
		Entities: tagMap,
	}

	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, _ := json.Marshal(resp)
		w.Write(out)
	}))
	defer s.Close()

	dir, err := ioutil.TempDir("", "TestZipTaggerList")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	taggerListURL = s.URL
	zipTaggerList(dir, "")
	content, err := ioutil.ReadFile(filepath.Join(dir, "tagger-list.json"))
	if err != nil {
		log.Fatal(err)
	}

	assert.Contains(t, string(content), "random_entity_name")
	assert.Contains(t, string(content), "docker_source_name")
	assert.Contains(t, string(content), "docker_image:custom-agent:latest")
	assert.Contains(t, string(content), "image_name:custom-agent")
}
