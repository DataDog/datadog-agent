// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

func init() {
	registerGroup(&autodiscoveryGroup)
	registerGroup(&containerFilteringGroup)
}

var autodiscoveryGroup = Group{
	Name:        "Autodiscovery",
	Description: "See https://docs.datadoghq.com/agent/autodiscovery/ for more details.",
	Priority:    40,
	Options: []*Option{
		{
			Name:     "listeners", // custom unmarshalling type
			NoEnvvar: true,
			YAMLDesc: `
The listener detects new containers, to start checks. Choose the one best suited for your system.
For now, "auto" only handles docker autodetection`,
			YAMLExample: `
listeners:
  - name: auto
  - name: docker
  - name: kubelet
  - name: ecs`,
		},
	},
}

var containerFilteringGroup = Group{
	Name: "Container filtering",
	Description: `
You can exclude containers from Autodiscovery and metrics collection.
These containers will still count towards the "docker.containers.*" metrics,
but will not increase your per-container billing`,
	Priority: 41,
	Options: []*Option{
		{
			Name:         "exclude_pause_container",
			DefaultValue: true,
			Description:  "Ignore kubernetes/openshift pod pause containers",
		},
		{
			Name:         "ac_exclude",
			DefaultValue: []string{},
			Description:  "Exclusion list for container AD and monitoring",
			YAMLDesc: `
You can exclude containers per image name or container name.
Rules are regexp, include overrides exclude. Some examples:
`,
			YAMLExample: `
exclude all, except containers based on the 'ubuntu' image or the 'debian' image.
ac_exclude: ["image:.*"]
ac_include: ["image:ubuntu", "image:debian"]

include all, except containers based on the 'ubuntu' image.
ac_exclude: ["image:ubuntu"]
ac_include: []

exclude all debian images except containers with a name starting with 'frontend'.
ac_exclude: ["image:debian"]
ac_include: ["name:frontend.*"]

exclude docker cloud "infrastructure" containers:
ac_exclude: ["image:dockercloud/network-daemon","image:dockercloud/cleanup","image:dockercloud/logrotate","image:dockercloud/events","image:dockercloud/ntpd"]
ac_include: []`,
		},
		{
			Name:            "ac_include",
			DefaultValue:    []string{},
			Description:     "Inclusion whitelist for container AD and monitoring",
			SkipYAMLDesc:    true, // documentation merged with ac_exclude
			SkipYAMLExample: true, // documentation merged with ac_exclude
		},
	},
}
