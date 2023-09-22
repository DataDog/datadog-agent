// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	procmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/config"
	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func TestGoRoutines(t *testing.T) {
	expected := "No Goroutines for you, my friend!"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s", expected)
	}))
	defer ts.Close()

	content, err := getHTTPCallContent(ts.URL)
	require.NoError(t, err)
	assert.Equal(t, expected, string(content))
}

func TestIncludeSystemProbeConfig(t *testing.T) {
	common.SetupConfigWithWarnings("./test/datadog-agent.yaml", "")
	// create system-probe.yaml file because it's in .gitignore
	_, err := os.Create("./test/system-probe.yaml")
	require.NoError(t, err, "couldn't create system-probe.yaml")
	defer os.Remove("./test/system-probe.yaml")

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	getConfigFiles(mock.Fb, searchPaths{"": "./test/confd"})

	mock.AssertFileExists("etc", "datadog.yaml")
	mock.AssertFileExists("etc", "system-probe.yaml")
}

func TestIncludeConfigFiles(t *testing.T) {
	common.SetupConfigWithWarnings("./test", "")

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	getConfigFiles(mock.Fb, searchPaths{"": "./test/confd"})

	mock.AssertFileExists("etc/confd/test.yaml")
	mock.AssertFileExists("etc/confd/test.Yml")
	mock.AssertNoFileExists("etc/confd/not_included.conf")
}

func TestIncludeConfigFilesWithPrefix(t *testing.T) {
	common.SetupConfigWithWarnings("./test", "")

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	getConfigFiles(mock.Fb, searchPaths{"prefix": "./test/confd"})

	mock.AssertFileExists("etc/confd/prefix/test.yaml")
	mock.AssertFileExists("etc/confd/prefix/test.Yml")
	mock.AssertNoFileExists("etc/confd/prefix/not_included.conf")
}

func createTestFile(t *testing.T, filename string) string {
	path := filepath.Join(t.TempDir(), filename)
	require.NoError(t, os.WriteFile(path, []byte("mockfilecontent"), os.ModePerm))
	return path
}

func TestRegistryJSON(t *testing.T) {
	srcDir := createTestFile(t, "registry.json")

	confMock := config.Mock(t)
	confMock.Set("logs_config.run_path", filepath.Dir(srcDir))

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	getRegistryJSON(mock.Fb)

	mock.AssertFileContent("mockfilecontent", "registry.json")
}

func setupIPCAddress(t *testing.T, URL string) *config.MockConfig {
	u, err := url.Parse(URL)
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)

	confMock := config.Mock(t)
	confMock.Set("ipc_address", host)
	confMock.Set("cmd_port", port)
	confMock.Set("process_config.cmd_port", port)

	return confMock
}

func TestGetAgentTaggerList(t *testing.T) {
	tagMap := make(map[string]tagger_api.TaggerListEntity)
	tagMap["random_entity_name"] = tagger_api.TaggerListEntity{
		Tags: map[string][]string{
			"docker_source_name": {"docker_image:custom-agent:latest", "image_name:custom-agent"},
		},
	}
	resp := tagger_api.TaggerListResponse{
		Entities: tagMap,
	}

	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, _ := json.Marshal(resp)
		w.Write(out)
	}))
	defer s.Close()

	setupIPCAddress(t, s.URL)

	content, err := getAgentTaggerList()
	require.NoError(t, err)

	assert.Contains(t, string(content), "random_entity_name")
	assert.Contains(t, string(content), "docker_source_name")
	assert.Contains(t, string(content), "docker_image:custom-agent:latest")
	assert.Contains(t, string(content), "image_name:custom-agent")
}

func TestGetWorkloadList(t *testing.T) {
	workloadMap := make(map[string]workloadmeta.WorkloadEntity)
	workloadMap["kind_id"] = workloadmeta.WorkloadEntity{
		Infos: map[string]string{
			"container_id_1": "Name: init-volume ID: e19e1ba787",
			"container_id_2": "Name: init-config ID: 4e0ffee5d6",
			"container_id_3": "Name: init-passwd ID: f17539af8d pwd: admin123",
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

	setupIPCAddress(t, s.URL)

	content, err := getAgentWorkloadList()
	require.NoError(t, err)

	assert.Contains(t, string(content), "kind_id")
	assert.Contains(t, string(content), "container_id_1")
	assert.Contains(t, string(content), "Name: init-volume ID: e19e1ba787")
	assert.Contains(t, string(content), "container_id_2")
	assert.Contains(t, string(content), "Name: init-config ID: 4e0ffee5d6")
	assert.NotContains(t, string(content), "Name: init-passwd ID: f17539af8d pwd: admin123")
}

func TestVersionHistory(t *testing.T) {
	srcDir := createTestFile(t, "version-history.json")

	confMock := config.Mock(t)
	confMock.Set("run_path", filepath.Dir(srcDir))

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	getVersionHistory(mock.Fb)

	mock.AssertFileContent("mockfilecontent", "version-history.json")
}

func TestProcessAgentFullConfig(t *testing.T) {
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

	exp := `api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
dd_url: https://my-url.com
process_config:
  enabled: "true"
`

	t.Run("without process-agent running", func(t *testing.T) {
		content, err := getProcessAgentFullConfig()
		require.NoError(t, err)
		assert.Equal(t, "error: process-agent is not running or is unreachable\n", string(content))
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

		setupIPCAddress(t, srv.URL)

		content, err := getProcessAgentFullConfig()
		require.NoError(t, err)
		assert.Equal(t, exp, string(content))
	})
}

func TestProcessAgentChecks(t *testing.T) {
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
		mock := flarehelpers.NewFlareBuilderMock(t, false)
		getProcessChecks(mock.Fb, func() (string, error) { return "fake:1337", nil })

		mock.AssertFileContentMatch("error: process-agent is not running or is unreachable: error collecting data for 'process_discovery_check_output.json': .*", "process_check_output.json")
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

		setupIPCAddress(t, srv.URL)

		mock := flarehelpers.NewFlareBuilderMock(t, false)
		getProcessChecks(mock.Fb, config.GetProcessAPIAddressPort)

		mock.AssertFileContent(string(expectedProcessesJSON), "process_check_output.json")
		mock.AssertFileContent(string(expectedContainersJSON), "container_check_output.json")
		mock.AssertFileContent(string(expectedProcessDiscoveryJSON), "process_discovery_check_output.json")
	})
}
