// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

// Package tailerfactory implements the logic required to determine which kind
// of tailer to use for a container-related LogSource, and to create that tailer.
package tailerfactory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	compConfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

var platformDockerLogsBasePath string

func fileTestSetup(t *testing.T) {
	tmp := t.TempDir()
	var oldPodLogsBasePath, oldDockerLogsBasePathNix, oldDockerLogsBasePathWin string
	oldPodLogsBasePath, podLogsBasePath = podLogsBasePath, filepath.Join(tmp, "pods")
	oldDockerLogsBasePathNix, dockerLogsBasePathNix = dockerLogsBasePathNix, filepath.Join(tmp, "docker-nix")
	oldDockerLogsBasePathWin, dockerLogsBasePathWin = dockerLogsBasePathWin, filepath.Join(tmp, "docker-win")

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
	})
}

func makeTestPod() (*workloadmeta.KubernetesPod, *workloadmeta.Container) {
	podID := "poduuid"
	containerID := "abc"
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			ID:   podID,
			Kind: workloadmeta.KindKubernetesPod,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "podname",
			Namespace: "podns",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   containerID,
				Name: "cname",
				Image: workloadmeta.ContainerImage{
					Name: "iname",
				},
			},
		},
	}

	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podID,
		},
	}

	return pod, container
}

func newWorkloadmetaMock(t *testing.T) workloadmetamock.Mock {
	t.Helper()

	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

func setPodmanContainerRootDir(wmeta workloadmetamock.Mock, containerID, rootDir string) {
	wmeta.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Annotations: map[string]string{
				pkglog.ContainerRootDirAnnotationKey: rootDir,
			},
		},
		Runtime: workloadmeta.ContainerRuntimePodman,
	})
}

func TestMakeFileSource_docker_success(t *testing.T) {
	fileTestSetup(t)

	p := filepath.Join(platformDockerLogsBasePath, filepath.FromSlash("containers/abc/abc-json.log"))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o777))
	require.NoError(t, os.WriteFile(p, []byte("{}"), 0o666))

	tf := &factory{
		pipelineProvider: pipeline.NewMockProvider(),
		cop:              containersorpods.NewDecidedChooser(containersorpods.LogContainers),
		dockerUtilGetter: &dockerUtilGetterImpl{},
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:                        "docker",
		Identifier:                  "abc",
		Source:                      "src",
		Service:                     "svc",
		Tags:                        []string{"tag!"},
		AutoMultiLine:               pointer.Ptr(true),
		AutoMultiLineSampleSize:     123,
		AutoMultiLineMatchThreshold: 0.123,
		ExperimentalAdaptiveSampling: &config.SourceAdaptiveSamplingOptions{
			Enabled: pointer.Ptr(true),
		},
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
	require.Equal(t, *source.Config.AutoMultiLine, true)
	require.Equal(t, source.Config.AutoMultiLineSampleSize, 123)
	require.Equal(t, source.Config.AutoMultiLineMatchThreshold, 0.123)
	require.NotNil(t, child.Config.ExperimentalAdaptiveSampling)
	require.NotNil(t, child.Config.ExperimentalAdaptiveSampling.Enabled)
	require.True(t, *child.Config.ExperimentalAdaptiveSampling.Enabled)
}

func TestMakeFileSource_podman_success(t *testing.T) {
	fileTestSetup(t)
	tmp := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("logs_config.use_podman_logs", true)

	// On Windows, podman runs within a Linux virtual machine, so the Agent would believe it runs in a Linux environment with all the paths being nix-like.
	// The real path on the system is abstracted by the Windows Subsystem for Linux layer, so this unit test is skipped.
	// Ref: https://github.com/containers/podman/blob/main/docs/tutorials/podman-for-windows.md
	if runtime.GOOS == "windows" {
		t.Skip("Skip on Windows due to WSL file path abstraction")
	}

	containersRoot := filepath.Join(tmp, "containers")
	p := filepath.Join(containersRoot, filepath.FromSlash("storage/overlay-containers/abc/userdata/ctr.log"))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o777))
	require.NoError(t, os.WriteFile(p, []byte("{}"), 0o666))

	wmeta := newWorkloadmetaMock(t)
	setPodmanContainerRootDir(wmeta, "abc", containersRoot)

	tf := &factory{
		pipelineProvider:  pipeline.NewMockProvider(),
		cop:               containersorpods.NewDecidedChooser(containersorpods.LogContainers),
		dockerUtilGetter:  &dockerUtilGetterImpl{},
		workloadmetaStore: option.New[workloadmeta.Component](wmeta),
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:                        "podman",
		Identifier:                  "abc",
		Source:                      "src",
		Service:                     "svc",
		Tags:                        []string{"tag!"},
		AutoMultiLine:               pointer.Ptr(true),
		AutoMultiLineSampleSize:     321,
		AutoMultiLineMatchThreshold: 0.321,
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
	require.Equal(t, *source.Config.AutoMultiLine, true)
	require.Equal(t, source.Config.AutoMultiLineSampleSize, 321)
	require.Equal(t, source.Config.AutoMultiLineMatchThreshold, 0.321)
}

func TestMakeFileSource_podman_with_db_path_uses_annotation_success(t *testing.T) {
	tmp := t.TempDir()
	customPath := filepath.Join(tmp, "/configured/path/containers/storage/db.sql")
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("logs_config.use_podman_logs", true)
	mockConfig.SetWithoutSource("podman_db_path", customPath)

	// On Windows, podman runs within a Linux virtual machine, so the Agent would believe it runs in a Linux environment with all the paths being nix-like.
	// The real path on the system is abstracted by the Windows Subsystem for Linux layer, so this unit test is skipped.
	// Ref: https://github.com/containers/podman/blob/main/docs/tutorials/podman-for-windows.md
	if runtime.GOOS == "windows" {
		t.Skip("Skip on Windows due to WSL file path abstraction")
	}

	actualContainersRoot := filepath.Join(tmp, "/actual/path/containers")
	p := filepath.Join(actualContainersRoot, filepath.FromSlash("storage/overlay-containers/abc/userdata/ctr.log"))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o777))
	require.NoError(t, os.WriteFile(p, []byte("{}"), 0o666))

	wmeta := newWorkloadmetaMock(t)
	setPodmanContainerRootDir(wmeta, "abc", actualContainersRoot)

	tf := &factory{
		pipelineProvider:  pipeline.NewMockProvider(),
		cop:               containersorpods.NewDecidedChooser(containersorpods.LogContainers),
		dockerUtilGetter:  &dockerUtilGetterImpl{},
		workloadmetaStore: option.New[workloadmeta.Component](wmeta),
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "podman",
		Identifier: "abc",
		Source:     "src",
		Service:    "svc",
	})
	child, err := tf.makeFileSource(source)
	require.NoError(t, err)
	require.Equal(t, source.Name, child.Name)
	require.Equal(t, "file", child.Config.Type)
	require.Equal(t, source.Config.Identifier, child.Config.Identifier)
	require.Equal(t, p, child.Config.Path)
	require.Equal(t, source.Config.Source, child.Config.Source)
	require.Equal(t, source.Config.Service, child.Config.Service)
	require.Equal(t, sources.DockerSourceType, child.GetSourceType())
}

func TestMakeFileSource_podman_with_multiple_db_paths_success(t *testing.T) {
	tmp := t.TempDir()

	// On Windows, podman runs within a Linux virtual machine, so the Agent would believe it runs in a Linux environment with all the paths being nix-like.
	// The real path on the system is abstracted by the Windows Subsystem for Linux layer, so this unit test is skipped.
	// Ref: https://github.com/containers/podman/blob/main/docs/tutorials/podman-for-windows.md
	if runtime.GOOS == "windows" {
		t.Skip("Skip on Windows due to WSL file path abstraction")
	}

	// First DB path (root user)
	rootDBPath := filepath.Join(tmp, "root/containers/storage/db.sql")

	// Second DB path (regular user)
	userContainersRoot := filepath.Join(tmp, "user/containers")
	userDBPath := filepath.Join(userContainersRoot, "storage/db.sql")
	userLogPath := filepath.Join(userContainersRoot, "storage/overlay-containers/abc/userdata/ctr.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(userLogPath), 0o777))
	require.NoError(t, os.WriteFile(userLogPath, []byte("{}"), 0o666))

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("logs_config.use_podman_logs", true)
	mockConfig.SetWithoutSource("podman_db_path", rootDBPath+","+userDBPath)

	wmeta := newWorkloadmetaMock(t)
	setPodmanContainerRootDir(wmeta, "abc", userContainersRoot)

	tf := &factory{
		pipelineProvider:  pipeline.NewMockProvider(),
		cop:               containersorpods.NewDecidedChooser(containersorpods.LogContainers),
		dockerUtilGetter:  &dockerUtilGetterImpl{},
		workloadmetaStore: option.New[workloadmeta.Component](wmeta),
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "podman",
		Identifier: "abc",
		Source:     "src",
		Service:    "svc",
	})
	child, err := tf.makeFileSource(source)
	require.NoError(t, err)
	require.Equal(t, userLogPath, child.Config.Path)
}

func TestMakeFileSource_podman_without_annotation_errors_even_with_dbpath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skip on Windows due to WSL file path abstraction")
	}

	tmp := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("logs_config.use_podman_logs", true)
	mockConfig.SetWithoutSource("podman_db_path", filepath.Join(tmp, "configured/path/containers/storage/db.sql"))

	wmeta := newWorkloadmetaMock(t)
	wmeta.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "abc",
		},
		Runtime: workloadmeta.ContainerRuntimePodman,
	})

	tf := &factory{
		pipelineProvider:  pipeline.NewMockProvider(),
		cop:               containersorpods.NewDecidedChooser(containersorpods.LogContainers),
		dockerUtilGetter:  &dockerUtilGetterImpl{},
		workloadmetaStore: option.New[workloadmeta.Component](wmeta),
	}

	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "podman",
		Identifier: "abc",
		Source:     "src",
		Service:    "svc",
	})

	_, err := tf.makeFileSource(source)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing annotation")
}

// TestMakeFileSource_podman_autodiscovery_home_user verifies that when podman_db_path is empty
// (auto-discovery mode), log collection uses the root dir stored by the collector in workloadmeta.
func TestMakeFileSource_podman_autodiscovery_home_user(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skip on Windows due to WSL file path abstraction")
	}

	tmp := t.TempDir()

	// Simulate a rootless user's storage root and log file.
	userContainersRoot := filepath.Join(tmp, "home", "testuser", ".local", "share", "containers")
	userLogPath := filepath.Join(userContainersRoot, "storage", "overlay-containers", "abc", "userdata", "ctr.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(userLogPath), 0o777))
	require.NoError(t, os.WriteFile(userLogPath, []byte("{}"), 0o666))

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("logs_config.use_podman_logs", true)
	// podman_db_path is intentionally left empty (auto-discovery mode)

	// Populate a workloadmeta store with the container annotated with its root dir,
	// as the Podman collector would have done at collection time.
	wmeta := newWorkloadmetaMock(t)
	setPodmanContainerRootDir(wmeta, "abc", userContainersRoot)

	tf := &factory{
		pipelineProvider:  pipeline.NewMockProvider(),
		cop:               containersorpods.NewDecidedChooser(containersorpods.LogContainers),
		dockerUtilGetter:  &dockerUtilGetterImpl{},
		workloadmetaStore: option.New[workloadmeta.Component](wmeta),
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "podman",
		Identifier: "abc",
		Source:     "src",
		Service:    "svc",
	})
	child, err := tf.makeFileSource(source)
	require.NoError(t, err)
	require.Equal(t, userLogPath, child.Config.Path)
}

func TestMakeFileSource_docker_no_file(t *testing.T) {
	fileTestSetup(t)

	p := filepath.Join(platformDockerLogsBasePath, filepath.FromSlash("containers/abc/abc-json.log"))

	tf := &factory{
		pipelineProvider: pipeline.NewMockProvider(),
		cop:              containersorpods.NewDecidedChooser(containersorpods.LogContainers),
		dockerUtilGetter: &dockerUtilGetterImpl{},
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
	mockConfig := configmock.New(t)
	customPath := filepath.Join(tmp, "/custom/path")
	mockConfig.SetWithoutSource("logs_config.docker_path_override", customPath)

	p := filepath.Join(mockConfig.GetString("logs_config.docker_path_override"), filepath.FromSlash("containers/abc/abc-json.log"))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o777))
	require.NoError(t, os.WriteFile(p, []byte("{}"), 0o666))

	tf := &factory{
		pipelineProvider: pipeline.NewMockProvider(),
		cop:              containersorpods.NewDecidedChooser(containersorpods.LogContainers),
		dockerUtilGetter: &dockerUtilGetterImpl{},
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "docker",
		Identifier: "abc",
		Source:     "src",
		Service:    "svc",
	})

	_, err := tf.findDockerLogPath(source.Config.Identifier)
	require.NoError(t, err)

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

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	pod, container := makeTestPod()
	store.Set(pod)
	store.Set(container)

	tf := &factory{
		pipelineProvider:  pipeline.NewMockProvider(),
		cop:               containersorpods.NewDecidedChooser(containersorpods.LogPods),
		dockerUtilGetter:  &dockerUtilGetterImpl{},
		workloadmetaStore: option.New[workloadmeta.Component](store),
	}
	for _, sourceConfigType := range []string{"docker", "containerd"} {
		t.Run("source.Config.Type="+sourceConfigType, func(t *testing.T) {
			source := sources.NewLogSource("test", &config.LogsConfig{
				Type:                        sourceConfigType,
				Identifier:                  "abc",
				Source:                      "src",
				Service:                     "svc",
				Tags:                        []string{"tag!"},
				AutoMultiLine:               pointer.Ptr(true),
				AutoMultiLineSampleSize:     123,
				AutoMultiLineMatchThreshold: 0.123,
				ExperimentalAdaptiveSampling: &config.SourceAdaptiveSamplingOptions{
					Enabled: pointer.Ptr(false),
				},
			})
			child, err := tf.makeK8sFileSource(source)
			require.NoError(t, err)
			require.Equal(t, "podns/podname/cname", child.Name)
			require.Equal(t, "file", child.Config.Type)
			require.Equal(t, "abc", child.Config.Identifier)
			require.Equal(t, wildcard, child.Config.Path)
			require.Equal(t, "src", child.Config.Source)
			require.Equal(t, "svc", child.Config.Service)
			require.Equal(t, []string{"tag!"}, []string(child.Config.Tags))
			require.Equal(t, *child.Config.AutoMultiLine, true)
			require.Equal(t, child.Config.AutoMultiLineSampleSize, 123)
			require.Equal(t, child.Config.AutoMultiLineMatchThreshold, 0.123)
			require.NotNil(t, child.Config.ExperimentalAdaptiveSampling)
			require.NotNil(t, child.Config.ExperimentalAdaptiveSampling.Enabled)
			require.False(t, *child.Config.ExperimentalAdaptiveSampling.Enabled)
			switch sourceConfigType {
			case "docker":
				require.Equal(t, sources.DockerSourceType, child.GetSourceType())
			case "containerd":
				require.Equal(t, sources.KubernetesSourceType, child.GetSourceType())
			}
		})
	}
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
			pod, _ := makeTestPod()
			gotPattern := findK8sLogPath(pod, "cname")
			require.Equal(t, filepath.Join(podLogsBasePath, expectedPattern), gotPattern)
		})
	}
}

func TestGetPodAndContainer_wmeta_not_initialize(t *testing.T) {
	tf := &factory{}
	container, pod, err := tf.getPodAndContainer("abc")

	require.Nil(t, container)
	require.Nil(t, pod)
	require.ErrorContains(t, err, "workloadmeta store is not initialized")

}

func TestGetPodAndContainer_pod_not_found(t *testing.T) {
	workloadmetaStore := fxutil.Test[option.Option[workloadmeta.Component]](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	tf := &factory{workloadmetaStore: workloadmetaStore}

	container, pod, err := tf.getPodAndContainer("abc")

	require.Nil(t, container)
	require.Nil(t, pod)
	require.ErrorContains(t, err, "cannot find pod for container")
}

// fullyPopulatedLogsConfig returns a LogsConfig with every exported, settable
// field set to a distinguishable non-zero value. The reflection walk ensures
// that newly added fields are automatically populated without manual updates.
func fullyPopulatedLogsConfig() *config.LogsConfig {
	cfg := &config.LogsConfig{}
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}
		setNonZero(field, t.Field(i).Name)
	}
	return cfg
}

// setNonZero sets a reflect.Value to a non-zero sentinel appropriate for its kind.
func setNonZero(v reflect.Value, name string) {
	switch v.Kind() {
	case reflect.String:
		v.SetString(name + "_val")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(42)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(42)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(0.42)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Ptr:
		elem := reflect.New(v.Type().Elem())
		setNonZero(elem.Elem(), name)
		v.Set(elem)
	case reflect.Slice:
		elemType := v.Type().Elem()
		elem := reflect.New(elemType).Elem()
		setNonZero(elem, name)
		v.Set(reflect.Append(reflect.MakeSlice(v.Type(), 0, 1), elem))
	case reflect.Chan:
		v.Set(reflect.MakeChan(v.Type(), 1))
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.CanSet() {
				setNonZero(f, v.Type().Field(i).Name)
			}
		}
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
	}
}

// TestLogsConfigFieldCoverage verifies that every field on LogsConfig is either
// copied by the container-to-file translation functions or explicitly listed in
// the exclusion set. When a new field is added to LogsConfig, this test fails
// unless the developer either copies it or adds it to the exclusion list.
func TestLogsConfigFieldCoverage(t *testing.T) {
	// Fields intentionally NOT copied from container source to file source.
	// If you add a new field to LogsConfig, you MUST either:
	//   (a) copy it in makeDockerFileSource and makeK8sFileSource, or
	//   (b) add it here with a comment explaining why it doesn't apply to file tailing.
	excludedFromFileCopy := map[string]string{
		// Hardcoded or computed differently per function
		"Type":   "hardcoded to FileType",
		"Path":   "computed from container ID / pod metadata",
		"Source": "computed via defaultSourceAndService",

		// Network fields (tcp/udp only)
		"Port":           "network source only",
		"BindHost":       "network source only",
		"IdleTimeout":    "network source only",
		"MaxConnections": "network source only",
		"TLS":            "network source only",
		"AllowedIPs":     "network source only",
		"DeniedIPs":      "network source only",

		// Journald fields
		"ConfigID":               "journald only",
		"IncludeSystemUnits":     "journald only",
		"ExcludeSystemUnits":     "journald only",
		"IncludeUserUnits":       "journald only",
		"ExcludeUserUnits":       "journald only",
		"IncludeMatches":         "journald only",
		"ExcludeMatches":         "journald only",
		"ContainerMode":          "journald only",
		"DefaultApplicationName": "journald only",

		// Docker identity fields (container metadata, not log content config)
		"Image": "docker container identity, not log config",
		"Label": "docker container identity, not log config",
		"Name":  "docker container identity, not log config",

		// Windows Event fields
		"ChannelPath": "windows event only",
		"Query":       "windows event only",

		// Channel tailer fields
		"Channel":          "internal channel tailer only",
		"ChannelTags":      "internal channel tailer only",
		"ChannelTagsMutex": "internal channel tailer only (sync.Mutex)",

		// File-only fields not relevant to container-sourced file tailing
		"ExcludePaths": "file wildcard exclusion, not applicable to container log paths",

		// Integration metadata (set by integration config loader, not user config)
		"IntegrationName":        "integration loader metadata",
		"IntegrationSource":      "integration loader metadata",
		"IntegrationSourceIndex": "integration loader metadata",

		// Structured-log tailer fields (journald, windows event only)
		"ProcessRawMessage": "only affects structured-message tailers (journald, windows event); file tailer always emits unstructured messages",

		// Misc fields not relevant to container-to-file
		"SourceCategory": "deprecated/unused in container context",
	}

	inputCfg := fullyPopulatedLogsConfig()

	// Override fields that the functions expect specific values for
	inputCfg.Type = "docker"
	inputCfg.Identifier = "abc"

	fileTestSetup(t)

	// Create the docker log file so makeDockerFileSource can open it
	dockerPath := filepath.Join(platformDockerLogsBasePath, filepath.FromSlash("containers/abc/abc-json.log"))
	require.NoError(t, os.MkdirAll(filepath.Dir(dockerPath), 0o777))
	require.NoError(t, os.WriteFile(dockerPath, []byte("{}"), 0o666))

	// Set up K8s workloadmeta so makeK8sFileSource can resolve the pod
	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() compConfig.Component { return compConfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	pod, container := makeTestPod()
	store.Set(pod)
	store.Set(container)

	// Create the K8s log directory
	k8sDir := filepath.Join(podLogsBasePath, filepath.FromSlash("podns_podname_poduuid/cname"))
	require.NoError(t, os.MkdirAll(k8sDir, 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(k8sDir, "0.log"), []byte("{}"), 0o666))

	type testCase struct {
		name       string
		makeSource func(*sources.LogSource) (*sources.LogSource, error)
	}

	dockerFactory := &factory{
		pipelineProvider: pipeline.NewMockProvider(),
		cop:              containersorpods.NewDecidedChooser(containersorpods.LogContainers),
		dockerUtilGetter: &dockerUtilGetterImpl{},
	}

	k8sFactory := &factory{
		pipelineProvider:  pipeline.NewMockProvider(),
		cop:               containersorpods.NewDecidedChooser(containersorpods.LogPods),
		dockerUtilGetter:  &dockerUtilGetterImpl{},
		workloadmetaStore: option.New[workloadmeta.Component](store),
	}

	cases := []testCase{
		{"makeDockerFileSource", dockerFactory.makeDockerFileSource},
		{"makeK8sFileSource", k8sFactory.makeK8sFileSource},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := sources.NewLogSource("test", inputCfg)
			child, err := tc.makeSource(source)
			require.NoError(t, err)

			checkFieldCoverage(t, inputCfg, child.Config, excludedFromFileCopy)
		})
	}
}

// checkFieldCoverage iterates over every exported field on LogsConfig and
// verifies that non-zero input fields are either non-zero on the output or
// present in the exclusion set. It also checks for stale exclusion entries.
func checkFieldCoverage(t *testing.T, input, output *config.LogsConfig, excluded map[string]string) {
	t.Helper()

	inVal := reflect.ValueOf(input).Elem()
	outVal := reflect.ValueOf(output).Elem()
	structType := inVal.Type()

	allFields := make(map[string]bool, structType.NumField())
	var missing []string

	for i := 0; i < structType.NumField(); i++ {
		fieldName := structType.Field(i).Name
		allFields[fieldName] = true

		inField := inVal.Field(i)
		outField := outVal.Field(i)

		if !inField.CanInterface() || !outField.CanInterface() {
			continue
		}

		if _, ok := excluded[fieldName]; ok {
			continue
		}

		if inField.IsZero() {
			continue
		}

		if outField.IsZero() {
			missing = append(missing, fieldName)
		}
	}

	if len(missing) > 0 {
		t.Errorf("LogsConfig fields are set on the container source but missing from the file source output.\n"+
			"Either copy them in makeDockerFileSource/makeK8sFileSource, or add them to\n"+
			"excludedFromFileCopy in TestLogsConfigFieldCoverage with a reason.\n"+
			"Missing fields: %v", missing)
	}

	for fieldName := range excluded {
		if !allFields[fieldName] {
			t.Errorf("Stale entry in excludedFromFileCopy: %q is no longer a field on LogsConfig. Remove it.", fieldName)
		}
	}
}

// TestLogsConfigFieldCoverage_detectsMissingField is a meta-test that verifies
// the guard logic catches a newly added field that is set on input but missing
// from output. It simulates the bug by building an output config that
// deliberately omits Encoding, then checks that Encoding appears in the
// missing-fields list.
func TestLogsConfigFieldCoverage_detectsMissingField(t *testing.T) {
	excluded := map[string]string{
		"Type": "hardcoded", "Path": "computed", "Source": "computed", "Service": "computed",
		"Port": "n/a", "BindHost": "n/a", "IdleTimeout": "n/a", "MaxConnections": "n/a",
		"TLS": "n/a", "AllowedIPs": "n/a", "DeniedIPs": "n/a",
		"ConfigID": "n/a", "IncludeSystemUnits": "n/a", "ExcludeSystemUnits": "n/a",
		"IncludeUserUnits": "n/a", "ExcludeUserUnits": "n/a",
		"IncludeMatches": "n/a", "ExcludeMatches": "n/a",
		"ContainerMode": "n/a", "DefaultApplicationName": "n/a",
		"Image": "n/a", "Label": "n/a", "Name": "n/a",
		"ChannelPath": "n/a", "Query": "n/a",
		"Channel": "n/a", "ChannelTags": "n/a", "ChannelTagsMutex": "n/a",
		"ExcludePaths":    "n/a",
		"IntegrationName": "n/a", "IntegrationSource": "n/a", "IntegrationSourceIndex": "n/a",
		"SourceCategory": "n/a",
	}

	input := fullyPopulatedLogsConfig()
	output := &config.LogsConfig{
		Type:            config.FileType,
		TailingMode:     input.TailingMode,
		Identifier:      input.Identifier,
		Path:            "some/path",
		Service:         input.Service,
		Source:          input.Source,
		Tags:            input.Tags,
		ProcessingRules: input.ProcessingRules,
		FingerprintConfig: &types.FingerprintConfig{
			FingerprintStrategy: "md5",
		},
	}

	inVal := reflect.ValueOf(input).Elem()
	outVal := reflect.ValueOf(output).Elem()
	structType := inVal.Type()

	var missing []string
	for i := 0; i < structType.NumField(); i++ {
		fieldName := structType.Field(i).Name
		inField := inVal.Field(i)
		outField := outVal.Field(i)
		if !inField.CanInterface() || !outField.CanInterface() {
			continue
		}
		if _, ok := excluded[fieldName]; ok {
			continue
		}
		if !inField.IsZero() && outField.IsZero() {
			missing = append(missing, fieldName)
		}
	}

	require.NotEmpty(t, missing, "Expected to detect missing fields")
	found := false
	for _, name := range missing {
		if name == "Encoding" {
			found = true
			break
		}
	}
	require.True(t, found, fmt.Sprintf("Expected 'Encoding' in missing fields, got: %v", missing))
}
