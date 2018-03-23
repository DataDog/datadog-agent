// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package config

import "os"

func init() {
	registerGroup(&dockerTaggingGroup)
	registerGroup(&dockerMetricsGroup)

	// Autodetect the default values for dockerMetricsGroup
	if IsContainerized() {
		Datadog.SetDefault("container_proc_root", "/host/proc")
		Datadog.SetDefault("container_cgroup_root", "/host/sys/fs/cgroup/")
	} else {
		Datadog.SetDefault("container_proc_root", "/proc")
		// for amazon linux the cgroup directory on host is /cgroup/
		// we pick memory.stat to make sure it exists and not empty
		if _, err := os.Stat("/cgroup/memory/memory.stat"); !os.IsNotExist(err) {
			Datadog.SetDefault("container_cgroup_root", "/cgroup/")
		} else {
			Datadog.SetDefault("container_cgroup_root", "/sys/fs/cgroup/")
		}
	}
}

var dockerMetricsGroup = Group{
	Name:        "Docker performance metrics",
	Description: "These settings impact the live container view and the docker check",
	Priority:    50,
	Options: []*Option{
		{
			Name:            "container_proc_root",
			DefaultAsString: "autodetected",
			Description:     "Folder where the host's proc filesystem is accessible",
		},
		{
			Name:            "container_cgroup_root",
			DefaultAsString: "autodetected",
			Description:     "Folder where the host's cgroup filesystem is accessible",
		},
	},
}

var dockerTaggingGroup = Group{
	Name:     "Docker tag extraction",
	Priority: 30,
	Options: []*Option{
		{
			Name:         "docker_labels_as_tags",
			DefaultValue: map[string]string{},
			Description:  "Container labels to extract as tags",
			YAMLDesc: `
The agent can extract container label or environment variables
as metric tags. If you prefix your tag name with +, it
will only be added to high cardinality metrics (docker check)
`,
			YAMLExample: `
docker_labels_as_tags:
  com.docker.compose.service: service_name
  com.docker.compose.project: +project_name`,
		},
		{
			Name:         "docker_env_as_tags",
			DefaultValue: map[string]string{},
			Description:  "Container environment variables to extract as tags",
			SkipYAMLDesc: true,
			YAMLExample: `
docker_env_as_tags:
  ENVVAR_NAME: tag_name`,
		},
	},
}
