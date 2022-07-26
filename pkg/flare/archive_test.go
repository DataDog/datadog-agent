// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	procmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func createTestDirStructure(
	t *testing.T,
	filename string,
) (string, string, error) {

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// create non-empty file in the source directory
	file, err := os.Create(filepath.Join(srcDir, filename))
	if err != nil {
		return "", "", err
	}

	_, err = file.WriteString("mockfilecontent")
	if err != nil {
		return "", "", err
	}

	err = file.Close()
	if err != nil {
		return "", "", err
	}

	return srcDir, dstDir, nil
}

func TestArchiveName(t *testing.T) {

	//test with No log level set
	zipFilePath := getArchivePath()
	assert.Contains(t, zipFilePath, "Z.zip")
	assert.NotContains(t, zipFilePath, "info")

	// init and configure logger at runtime
	config.SetupLogger("TEST", "debug", "", "", true, true, true)
	ll := settings.LogLevelRuntimeSetting{}

	// set 'trace' level logging
	err := ll.Set("trace")
	assert.Nil(t, err)

	// Verify the runtime setting is set to 'trace'
	v, err := ll.Get()
	assert.Equal(t, "trace", v)
	assert.Nil(t, err)

	// verify filePath string ends with the correct log_level
	zipFilePath = getArchivePath()
	assert.Contains(t, zipFilePath, "-trace.zip")
	assert.NotContains(t, zipFilePath, "Z.zip")
}

func TestCreateArchive(t *testing.T) {
	common.SetupConfig("./test")
	mockConfig := config.Mock(t)
	mockConfig.Set("confd_path", "./test/confd")
	mockConfig.Set("log_file", "./test/logs/agent.log")
	zipFilePath := getArchivePath()
	filePath, err := createArchive(SearchPaths{}, true, zipFilePath, []string{""}, nil, nil)

	require.Nil(t, err)
	require.Equal(t, zipFilePath, filePath)

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
	filePath, err := createArchive(SearchPaths{}, true, zipFilePath, []string{""}, nil, nil)

	require.Nil(t, err)
	require.Equal(t, zipFilePath, filePath)

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
	filePath, err := createArchive(SearchPaths{}, true, zipFilePath, []string{""}, nil, nil)

	require.Nil(t, err)
	require.Equal(t, zipFilePath, filePath)

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

	dir := t.TempDir()

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
	filePath, err := createArchive(SearchPaths{"": "./test/confd"}, true, zipFilePath, []string{""}, nil, nil)
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
	filePath, err := createArchive(SearchPaths{"": "./test/confd"}, true, zipFilePath, []string{""}, nil, nil)

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

func TestZipLogFiles(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	var err error
	f, err := os.Create(filepath.Join(srcDir, "agent.log"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	f, err = os.Create(filepath.Join(srcDir, "trace-agent.log"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	err = os.Mkdir(filepath.Join(srcDir, "archive"), 0700)
	require.NoError(t, err)

	f, err = os.Create(filepath.Join(srcDir, "archive", "agent.log"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	permsInfos := make(permissionsInfos)

	err = zipLogFiles(dstDir, "test", filepath.Join(srcDir, "agent.log"), permsInfos)
	require.NoError(t, err)

	// Check all the log files are in the destination path, at the right subdirectories
	_, err = os.Stat(filepath.Join(dstDir, "test", "logs", "agent.log"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dstDir, "test", "logs", "trace-agent.log"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dstDir, "test", "logs", "archive", "agent.log"))
	assert.NoError(t, err)
}

func TestZipRegistryJSON(t *testing.T) {
	srcDir, dstDir, err := createTestDirStructure(t, "registry.json")
	require.NoError(t, err)

	tempRunPath := config.Datadog.GetString("logs_config.run_path")
	config.Datadog.Set("logs_config.run_path", srcDir)
	defer config.Datadog.Set("logs_config.run_path", tempRunPath)

	err = zipRegistryJSON(dstDir, "test")
	require.NoError(t, err)

	targetPath := filepath.Join(dstDir, "test", "registry.json")
	actualContent, err := ioutil.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, "mockfilecontent", string(actualContent))
}

func TestZipTaggerList(t *testing.T) {
	tagMap := make(map[string]response.TaggerListEntity)
	tagMap["random_entity_name"] = response.TaggerListEntity{
		Tags: map[string][]string{
			"docker_source_name": {"docker_image:custom-agent:latest", "image_name:custom-agent"},
		},
	}
	resp := response.TaggerListResponse{
		Entities: tagMap,
	}

	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, _ := json.Marshal(resp)
		w.Write(out)
	}))
	defer s.Close()

	dir := t.TempDir()

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

func TestZipWorkloadList(t *testing.T) {
	workloadMap := make(map[string]workloadmeta.WorkloadEntity)
	workloadMap["kind_id"] = workloadmeta.WorkloadEntity{
		Infos: map[string]string{
			"container_id_1": "Name: init-volume ID: e19e1ba787",
			"container_id_2": "Name: init-config ID: 4e0ffee5d6",
		},
	}
	resp := workloadmeta.WorkloadDumpResponse{
		Entities: workloadMap,
	}

	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, _ := json.Marshal(resp)
		w.Write(out)
	}))
	defer s.Close()

	dir := t.TempDir()

	workloadListURL = s.URL
	zipWorkloadList(dir, "")
	content, err := ioutil.ReadFile(filepath.Join(dir, "workload-list.log"))
	if err != nil {
		log.Fatal(err)
	}

	assert.Contains(t, string(content), "kind_id")
	assert.Contains(t, string(content), "container_id_1")
	assert.Contains(t, string(content), "Name: init-volume ID: e19e1ba787")
	assert.Contains(t, string(content), "container_id_2")
	assert.Contains(t, string(content), "Name: init-config ID: 4e0ffee5d6")
}

func TestPerformanceProfile(t *testing.T) {
	testProfile := ProfileData{
		"first":  []byte{},
		"second": []byte{},
		"third":  []byte{},
	}
	zipFilePath := getArchivePath()
	filePath, err := createArchive(SearchPaths{}, true, zipFilePath, []string{""}, testProfile, nil)

	require.NoError(t, err)
	require.Equal(t, zipFilePath, filePath)

	// Open a zip archive for reading.
	z, err := zip.OpenReader(zipFilePath)
	if err != nil {
		assert.Fail(t, "Unable to open the flare archive")
	}
	defer z.Close()
	defer os.Remove(zipFilePath)

	firstHeap, secondHeap, cpu := false, false, false
	for _, f := range z.File {
		switch path.Base(f.Name) {
		case "first":
			firstHeap = true
		case "second":
			secondHeap = true
		case "third":
			cpu = true
		}
	}

	assert.True(t, firstHeap, "first-heap.profile should've been included")
	assert.True(t, secondHeap, "second-heap.profile should've been included")
	assert.True(t, cpu, "cpu.profile should've been included")
}

// Test that writeScrubbedFile actually scrubs third-party API keys.
func TestRedactingOtherServicesApiKey(t *testing.T) {
	dir := t.TempDir()
	filename := path.Join(dir, "test.config")

	clear := `init_config:
instances:
- host: 127.0.0.1
  api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  port: 8082
  api_key: dGhpc2++lzM+XBhc3N3b3JkW113aXRo/c29tZWN]oYXJzMTIzCg==
  version: 4 # omit this line if you're running pdns_recursor version 3.x`
	redacted := `init_config:
instances:
- host: 127.0.0.1
  api_key: "***************************aaaaa"
  port: 8082
  api_key: "********"
  version: 4 # omit this line if you're running pdns_recursor version 3.x`

	err := writeScrubbedFile(filename, []byte(clear))
	require.NoError(t, err)

	got, err := ioutil.ReadFile(filename)
	require.NoError(t, err)
	assert.Equal(t, redacted, string(got))
}

func TestZipFile(t *testing.T) {
	srcDir, dstDir, err := createTestDirStructure(t, "test.json")
	require.NoError(t, err)

	err = zipFile(srcDir, dstDir, "test.json")
	require.NoError(t, err)

	targetPath := filepath.Join(dstDir, "test.json")
	actualContent, err := ioutil.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, "mockfilecontent", string(actualContent))
}

func TestZipVersionHistory(t *testing.T) {
	srcDir, dstDir, err := createTestDirStructure(t, "version-history.json")
	require.NoError(t, err)

	tempRunPath := config.Datadog.GetString("run_path")
	config.Datadog.Set("run_path", srcDir)
	defer config.Datadog.Set("run_path", tempRunPath)

	err = zipVersionHistory(dstDir, "test")
	require.NoError(t, err)

	targetPath := filepath.Join(dstDir, "test", "version-history.json")
	actualContent, err := ioutil.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, "mockfilecontent", string(actualContent))
}

func TestZipProcessAgentFullConfig(t *testing.T) {
	type ProcessConfig struct {
		Enabled string `yaml:"enabled"`
	}

	globalCfg := struct {
		Apikey     string        `yaml:"api_key"`
		DDurl      string        `yaml:"dd_url"`
		ProcessCfg ProcessConfig `yaml:"process_config"`
	}{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"https://my-url.com",
		ProcessConfig{
			"true",
		},
	}

	exp := `api_key: "***************************aaaaa"
dd_url: https://my-url.com
process_config:
  enabled: "true"`

	t.Run("without process-agent running", func(t *testing.T) {
		dir := t.TempDir()

		zipProcessAgentFullConfig(dir, "")
		content, err := ioutil.ReadFile(filepath.Join(dir, "process_agent_runtime_config_dump.yaml"))
		require.NoError(t, err)
		assert.Equal(t, "error: process-agent is not running or is unreachable", string(content))
	})

	t.Run("with process-agent running", func(t *testing.T) {
		// Create a server to mock process-agent /config/all endpoint
		handler := func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			b, err := yaml.Marshal(globalCfg)
			require.NoError(t, err)

			_, err = w.Write(b)
			require.NoError(t, err)
		}
		srv := httptest.NewServer(http.HandlerFunc(handler))
		defer srv.Close()

		dir := t.TempDir()

		procStatusURL = srv.URL
		zipProcessAgentFullConfig(dir, "")
		content, err := ioutil.ReadFile(filepath.Join(dir, "process_agent_runtime_config_dump.yaml"))
		require.NoError(t, err)
		assert.Equal(t, exp, string(content))
	})
}

func TestZipProcessAgentChecks(t *testing.T) {
	expectedProcesses := []procmodel.MessageBody{
		&procmodel.CollectorProc{
			Processes: []*procmodel.Process{
				{
					Pid: 1337,
				},
			},
		},
	}
	expectedProcessesJSON, err := json.Marshal(&expectedProcesses)
	require.NoError(t, err)

	expectedContainers := []procmodel.MessageBody{
		&procmodel.CollectorContainer{
			Containers: []*procmodel.Container{
				{
					Id: "yeet",
				},
			},
		},
	}
	expectedContainersJSON, err := json.Marshal(&expectedContainers)
	require.NoError(t, err)

	expectedProcessDiscoveries := []procmodel.MessageBody{
		&procmodel.CollectorProcDiscovery{
			ProcessDiscoveries: []*procmodel.ProcessDiscovery{
				{
					Pid: 9001,
				},
			},
		},
	}
	expectedProcessDiscoveryJSON, err := json.Marshal(&expectedProcessDiscoveries)
	require.NoError(t, err)

	t.Run("without process-agent running", func(t *testing.T) {
		dir := t.TempDir()
		fmt.Println(dir)

		err = zipProcessChecks(dir, "", func() (string, error) { return "fake:1337", nil })
		require.NoError(t, err)

		content, err := ioutil.ReadFile(filepath.Join(dir, "process_check_output.json"))
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(content), "error: process-agent is not running or is unreachable"))
	})
	t.Run("with process-agent running", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.enabled", true)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.process_discovery.enabled", true)

		handler := func(w http.ResponseWriter, r *http.Request) {
			var err error
			switch r.URL.Path {
			case "/check/process":
				_, err = w.Write(expectedProcessesJSON)
			case "/check/container":
				_, err = w.Write(expectedContainersJSON)
			case "/check/process_discovery":
				_, err = w.Write(expectedProcessDiscoveryJSON)
			default:
				t.Error("Unexpected url endpoint", r.URL.Path)
			}
			require.NoError(t, err)
		}

		srv := httptest.NewServer(http.HandlerFunc(handler))
		defer srv.Close()

		dir := t.TempDir()

		err = zipProcessChecks(dir, "", func() (string, error) { return strings.TrimPrefix(srv.URL, "http://"), nil })
		require.NoError(t, err)

		content, err := ioutil.ReadFile(filepath.Join(dir, "process_check_output.json"))
		require.NoError(t, err)
		assert.Equal(t, expectedProcessesJSON, content)

		content, err = ioutil.ReadFile(filepath.Join(dir, "container_check_output.json"))
		require.NoError(t, err)
		assert.Equal(t, expectedContainersJSON, content)

		content, err = ioutil.ReadFile(filepath.Join(dir, "process_discovery_check_output.json"))
		require.NoError(t, err)
		assert.Equal(t, expectedProcessDiscoveryJSON, content)
	})
}
