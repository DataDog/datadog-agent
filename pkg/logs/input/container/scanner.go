// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package container

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input/docker"
	"github.com/DataDog/datadog-agent/pkg/logs/input/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// NewScanner returns a new container scanner,
// by default it returns the docker scanner.
//
// When running in kubernetes, the kubernetes integration can be enabled either setting:
// - the environment variable `DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL` to true
// - the datadog.yaml configuration parameter `logs_config.container_collect_all` to true
// For now, it's not possible to enable the kubernetes integration using an integration file and
// it's not possible to define pod or container filters either.
// However, pod annotations are supported to enrich and process logs but not for discovery purpose.
// The current implementation of the kubernetes integration is still a work in progress and will be improved in the future.
func NewScanner(sources *config.LogSources, pp pipeline.Provider, auditor *auditor.Auditor) restart.Restartable {
	if config.LogsAgent.GetBool("logs_config.container_collect_all") {
		if scanner, err := kubernetes.NewScanner(sources); err == nil {
			return scanner
		}
	}
	return docker.NewScanner(sources, pp, auditor)
}
