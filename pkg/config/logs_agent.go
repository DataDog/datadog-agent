// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

func init() {
	registerGroup(&logsAgentGroup)
}

var logsAgentGroup = Group{
	Name:        "Logs Management",
	Description: "See https://docs.datadoghq.com/logs/ for more information",
	Priority:    80,
	Options: []*Option{
		{
			Name:         "logs_enabled",
			DefaultValue: false,
			Description:  "Logs collection is disabled by default, set to true to enable it",
		},
		{
			Name:         "log_enabled",
			DefaultValue: false,
			Undocumented: true, // deprecated, use logs_enabled instead
		},
		{
			Name:         "logset",
			DefaultValue: "",
			Undocumented: true, // deprecated
		},
		{
			Name:         "logs_config",
			Description:  "",
			NoEnvvar:     true,
			SkipYAMLDesc: true,
			SubOptions: []*Option{
				{
					Name:         "dd_url",
					DefaultValue: "intake.logs.datadoghq.com",
					Description:  "TODO",
				},
				{
					Name:         "dd_port",
					DefaultValue: int(10516),
					Description:  "TODO",
				},
				{
					Name:         "dev_mode_use_proto",
					DefaultValue: false,
					Description:  "TODO",
				},
				{
					Name:            "run_path",
					DefaultValue:    defaultRunPath,
					DefaultAsString: "autodetected",
					Description:     "TODO",
				},
				{
					Name:         "open_files_limit",
					DefaultValue: int(100),
					Description:  "TODO",
				},
				{
					Name:         "container_collect_all",
					DefaultValue: false,
					Description:  "TODO",
				},
			},
		},
	},
}
