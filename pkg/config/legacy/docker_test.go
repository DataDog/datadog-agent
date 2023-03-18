// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && linux

// As we compare some paths, running the tests on Linux only

package legacy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	dockerDaemonLegacyConf string = `
init_config:
  docker_root: /host/test

instances:
  - url: "unix://var/run/docker.sock"
    collect_events: false
    filtered_event_types:
       - 'top'
       - 'exec_start'
       - 'exec_create'
    collect_container_size: true
    custom_cgroups: false
    health_service_check_whitelist: []
    collect_container_count: true
    collect_volume_count: true
    collect_images_stats: false
    collect_image_size: true
    collect_disk_stats: true
    collect_exit_codes: true
    exclude: ["name:test", "container_name:some_image.*", "badly_formated", "image_name:some_image_2", "image:some_image_3"]
    include: ["unknown_key:test", "image:some_image_3"]
    tags: ["tag:value", "value"]
    ecs_tags: false
    performance_tags: ["container_name", "image_name", "image_tag", "docker_image", "container_command", "container_id", "test"]
    container_tags: ["image_name", "image_tag", "docker_image"]
    event_attributes_as_tags: ["signal"]
    capped_metrics:
      docker.cpu.user: 1000
      docker.cpu.system: 1000
    collect_labels_as_tags: ["test1", "test2"]
`

	dockerNewConf string = `instances:
- collect_container_size: true
  collect_container_size_frequency: 5
  collect_exit_codes: true
  ok_exit_codes: []
  collect_images_stats: false
  collect_image_size: true
  collect_disk_stats: true
  collect_volume_count: true
  tags:
  - tag:value
  - value
  capped_metrics:
    docker.cpu.system: 1000
    docker.cpu.user: 1000
  collect_events: false
  unbundle_events: false
  filtered_event_types:
  - top
  - exec_start
  - exec_create
  collected_event_types: []
`
)

func TestConvertDocker(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "docker_daemon.yaml")
	dst := filepath.Join(dir, "docker.yaml")

	err := os.WriteFile(src, []byte(dockerDaemonLegacyConf), 0640)
	require.Nil(t, err)

	configConverter := config.NewConfigConverter()
	err = ImportDockerConf(src, dst, true, configConverter)
	require.Nil(t, err)

	newConf, err := os.ReadFile(filepath.Join(dir, "docker.yaml"))
	require.Nil(t, err)

	assert.Equal(t, dockerNewConf, string(newConf))

	assert.Equal(t, true, config.Datadog.GetBool("exclude_pause_container"))
	assert.Equal(t, []string{"name:test", "name:some_image.*", "image:some_image_2", "image:some_image_3"},
		config.Datadog.GetStringSlice("ac_exclude"))
	assert.Equal(t, []string{"image:some_image_3"}, config.Datadog.GetStringSlice("ac_include"))

	assert.Equal(t, "/host/test/proc", config.Datadog.GetString("container_proc_root"))
	assert.Equal(t, "/host/test/sys/fs/cgroup", config.Datadog.GetString("container_cgroup_root"))
	assert.Equal(t, map[string]string{"test1": "test1", "test2": "test2"},
		config.Datadog.GetStringMapString("docker_labels_as_tags"))

	// test overwrite
	err = ImportDockerConf(src, dst, false, configConverter)
	require.NotNil(t, err)
	_, err = os.Stat(filepath.Join(dir, "docker.yaml.bak"))
	assert.True(t, os.IsNotExist(err))

	err = ImportDockerConf(src, dst, true, configConverter)
	require.Nil(t, err)
	_, err = os.Stat(filepath.Join(dir, "docker.yaml.bak"))
	assert.False(t, os.IsNotExist(err))
}
