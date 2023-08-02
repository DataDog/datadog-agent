// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

// defaultFlareFiles contains all the files that are included in the flare archive by default (no need for a specific configuration option)
var defaultFlareFiles = []string{
	"config-check.log",
	"connectivity.log",
	"diagnose.log",
	"docker_ps.log",
	"envvars.log",
	"go-routine-dump.log",
	"health.yaml",
	"install_info",
	"metadata_inventories.json",
	"metadata_v5.json",
	"process_agent_runtime_config_dump.yaml",
	"process-agent_tagger-list.json",
	"runtime_config_dump.yaml",
	"secrets.log",
	"status.log",
	"system_probe_runtime_config_dump.yaml",
	"tagger-list.json",
	"version-history.json",
	"workload-list.log",
}
