// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	dockerutilPkg "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

var platformDockerLogsBasePath string

func fileTestSetup(t *testing.T) {
	dockerutilPkg.EnableTestingMode()
	tmp := t.TempDir()
	var oldPodLogsBasePath, oldDockerLogsBasePathNix, oldDockerLogsBasePathWin, oldPodmanLogsBasePath string
	oldPodLogsBasePath, podLogsBasePath = podLogsBasePath, filepath.Join(tmp, "pods")
	oldDockerLogsBasePathNix, dockerLogsBasePathNix = dockerLogsBasePathNix, filepath.Join(tmp, "docker-nix")
	oldDockerLogsBasePathWin, dockerLogsBasePathWin = dockerLogsBasePathWin, filepath.Join(tmp, "docker-win")
	oldPodmanLogsBasePath, podmanLogsBasePath = podmanLogsBasePath, filepath.Join(tmp, "containers")

	switch runtime.GOOS {
	case "windows":
		platformDockerLogsBasePath = dockerLogsBasePathWin
	default: // linux, darwin
		platformDockerLogsBasePath = dockerLogsBasePathNix
	}

	t.Cleanup(func() {
		podLogsBasePath = oldPodLogsBasePath
		dockerLogsBasePathNix = oldDockerLogsBasePathNix
		dockerLogsBasePathWin = oldDockerLogsBasePathWin
		podmanLogsBasePath = oldPodmanLogsBasePath
	})
}

func makeTestPod() *workloadmeta.KubernetesPod {
	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			ID:   "poduuid",
			Kind: workloadmeta.KindKubernetesPod,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "podname",
			Namespace: "podns",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "abc",
				Name: "cname",
				Image: workloadmeta.ContainerImage{
					Name: "iname",
				},
			},
		},
	}
}

func TestMakeFileSource_docker_success(t *testing.T) {
	fileTestSetup(t)

	p := filepath.Join(platformDockerLogsBasePath, filepath.FromSlash("containers/abc/abc-json.log"))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o777))
	require.NoError(t, os.WriteFile(p, []byte("{}"), 0o666))

	tf := &factory{
		pipelineProvider: pipeline.NewMockProvider(),
		cop:              containersorpods.NewDecidedChooser(containersorpods.LogContainers),
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "docker",
		Identifier: "abc",
		Source:     "src",
		Service:    "svc",
		Tags:       []string{"tag!"},
	})
	child, err := tf.makeFileSource(source)
	require.NoError(t, err)
	require.Equal(t, source.Name, child.Name)
	require.Equal(t, "file", child.Config.Type)
	require.Equal(t, source.Config.Identifier, child.Config.Identifier)
	require.Equal(t, p, child.Config.Path)
	require.Equal(t, source.Config.Source, child.Config.Source)
	require.Equal(t, source.Config.Service, child.Config.Service)
	require.Equal(t, source.Config.Tags, child.Config.Tags)
	require.Equal(t, sources.DockerSourceType, child.GetSourceType())
}

func TestMakeFileSource_docker_no_file(t *testing.T) {
	fileTestSetup(t)

	p := filepath.Join(platformDockerLogsBasePath, filepath.FromSlash("containers/abc/abc-json.log"))

	tf := &factory{
		pipelineProvider: pipeline.NewMockProvider(),
		cop:              containersorpods.NewDecidedChooser(containersorpods.LogContainers),
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "docker",
		Identifier: "abc",
		Source:     "src",
		Service:    "svc",
	})
	child, err := tf.makeFileSource(source)
	require.Nil(t, child)
	require.Error(t, err)
	switch runtime.GOOS {
	case "windows":
		require.Contains(t, err.Error(), "The system cannot find the path specified")
	default: // linux, darwin
		require.Contains(t, err.Error(), p) // error is about the path
	}
}

func TestDockerOverride(t *testing.T) {
	tmp := t.TempDir()
	mockConfig := coreConfig.Mock(t)
	customPath := filepath.Join(tmp, "/custom/path")
	mockConfig.Set("logs_config.docker_path_override", customPath)

	p := filepath.Join(mockConfig.GetString("logs_config.docker_path_override"), filepath.FromSlash("containers/abc/abc-json.log"))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o777))
	require.NoError(t, os.WriteFile(p, []byte("{}"), 0o666))

	tf := &factory{
		pipelineProvider: pipeline.NewMockProvider(),
		cop:              containersorpods.NewDecidedChooser(containersorpods.LogContainers),
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "docker",
		Identifier: "abc",
		Source:     "src",
		Service:    "svc",
	})

	tf.findDockerLogPath(source.Config.Identifier)

	child, err := tf.makeFileSource(source)

	require.NoError(t, err)
	require.Equal(t, "file", child.Config.Type)
	require.Equal(t, p, child.Config.Path)
}

func TestMakeK8sSource(t *testing.T) {
	fileTestSetup(t)

	dir := filepath.Join(podLogsBasePath, filepath.FromSlash("podns_podname_poduuid/cname"))
	require.NoError(t, os.MkdirAll(dir, 0o777))
	filename := filepath.Join(dir, "somefile.log")
	require.NoError(t, os.WriteFile(filename, []byte("{}"), 0o666))
	wildcard := filepath.Join(dir, "*.log")

	store := workloadmeta.NewMockStore()
	store.SetEntity(makeTestPod())

	tf := &factory{
		pipelineProvider:  pipeline.NewMockProvider(),
		cop:               containersorpods.NewDecidedChooser(containersorpods.LogPods),
		workloadmetaStore: store,
	}
	for _, sourceConfigType := range []string{"docker", "containerd"} {
		t.Run("source.Config.Type="+sourceConfigType, func(t *testing.T) {
			source := sources.NewLogSource("test", &config.LogsConfig{
				Type:       sourceConfigType,
				Identifier: "abc",
				Source:     "src",
				Service:    "svc",
				Tags:       []string{"tag!"},
			})
			child, err := tf.makeK8sFileSource(source)
			require.NoError(t, err)
			require.Equal(t, "podns/podname/cname", child.Name)
			require.Equal(t, "file", child.Config.Type)
			require.Equal(t, "abc", child.Config.Identifier)
			require.Equal(t, wildcard, child.Config.Path)
			require.Equal(t, "src", child.Config.Source)
			require.Equal(t, "svc", child.Config.Service)
			require.Equal(t, []string{"tag!"}, child.Config.Tags)
			switch sourceConfigType {
			case "docker":
				require.Equal(t, sources.DockerSourceType, child.GetSourceType())
			case "containerd":
				require.Equal(t, sources.KubernetesSourceType, child.GetSourceType())
			}
		})
	}
}

func TestMakeK8sSource_pod_not_found(t *testing.T) {
	fileTestSetup(t)

	p := filepath.Join(platformDockerLogsBasePath, "containers/abc/abc-json.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o777))
	require.NoError(t, os.WriteFile(p, []byte("{}"), 0o666))

	tf := &factory{
		pipelineProvider:  pipeline.NewMockProvider(),
		cop:               containersorpods.NewDecidedChooser(containersorpods.LogPods),
		workloadmetaStore: workloadmeta.NewMockStore(),
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "docker",
		Identifier: "abc",
	})
	child, err := tf.makeK8sFileSource(source)
	require.Nil(t, child)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot find pod for container")
}

func TestFindK8sLogPath(t *testing.T) {
	fileTestSetup(t)

	tests := []struct{ name, pathExists, expectedPattern string }{
		{"..v1.9", "poduuid/cname_1.log", "poduuid/cname_*.log"},
		{"v1.10..v1.13", "poduuid/cname/1.log", "poduuid/cname/*.log"},
		{"v1.14..", "podns_podname_poduuid/cname/1.log", "podns_podname_poduuid/cname/*.log"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pathExists := filepath.FromSlash(test.pathExists)
			expectedPattern := filepath.FromSlash(test.expectedPattern)
			p := filepath.Join(podLogsBasePath, pathExists)
			require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o777))
			require.NoError(t, os.WriteFile(p, []byte("xx"), 0o666))
			defer func() {
				require.NoError(t, os.RemoveAll(podLogsBasePath))
			}()

			gotPattern := findK8sLogPath(makeTestPod(), "cname")
			require.Equal(t, filepath.Join(podLogsBasePath, expectedPattern), gotPattern)
		})
	}
}
