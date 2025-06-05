// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	filter "github.com/DataDog/datadog-agent/comp/core/filter/def"
	"github.com/DataDog/datadog-agent/comp/core/filter/program"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// ContainerMetricsProgram creates a program for filtering container metrics
func ContainerMetricsProgram(config config.Component, logger log.Component) program.CELProgram {
	includeList := config.GetStringSlice("container_include_metrics")
	excludeList := config.GetStringSlice("container_exclude_metrics")

	return program.CELProgram{
		Name:    "ContainerMetricsProgram",
		Include: createProgramFromOldFilters(includeList, filter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerType, logger),
	}
}

// ContainerLogsProgram creates a program for filtering container logs
func ContainerLogsProgram(config config.Component, logger log.Component) program.CELProgram {
	includeList := config.GetStringSlice("container_include_logs")
	excludeList := config.GetStringSlice("container_exclude_logs")

	return program.CELProgram{
		Name:    "ContainerLogsProgram",
		Include: createProgramFromOldFilters(includeList, filter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerType, logger),
	}
}

// ContainerACLegacyExcludeProgram creates a program for excluding containers via legacy `AC` filters
func ContainerACLegacyExcludeProgram(config config.Component, logger log.Component) program.CELProgram {
	excludeList := config.GetStringSlice("ac_exclude")

	return program.CELProgram{
		Name:    "ContainerACLegacyExcludeProgram",
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerType, logger),
	}
}

// ContainerACLegacyIncludeProgram creates a program for including containers via legacy `AC` filters
func ContainerACLegacyIncludeProgram(config config.Component, logger log.Component) program.CELProgram {
	includeList := config.GetStringSlice("ac_include")

	return program.CELProgram{
		Name:    "ContainerACLegacyIncludeProgram",
		Include: createProgramFromOldFilters(includeList, filter.ContainerType, logger),
	}
}

// ContainerGlobalProgram creates a program for filtering container globally
func ContainerGlobalProgram(config config.Component, logger log.Component) program.CELProgram {
	includeList := config.GetStringSlice("container_include")
	excludeList := config.GetStringSlice("container_exclude")

	return program.CELProgram{
		Name:    "ContainerGlobalProgram",
		Include: createProgramFromOldFilters(includeList, filter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerType, logger),
	}
}

// ContainerADAnnotationsProgram creates a program for filtering container annotations
func ContainerADAnnotationsProgram(_ config.Component, logger log.Component) program.CELProgram {
	excludeFilter := `("ad.datadoghq.com/" + container.name + ".exclude") in container.pod.annotations && container.pod.annotations["ad.datadoghq.com/" + container.name + ".exclude"] == "true"`
	excludeProgram, err := createCELProgram(excludeFilter, filter.ContainerType)

	if err != nil {
		logger.Warnf("Error creating CEL filtering program: %v", err)
	}

	return program.CELProgram{
		Name:    "ContainerADAnnotationsProgram",
		Exclude: excludeProgram,
	}
}

// ContainerPausedProgram creates a program for filtering paused containers
func ContainerPausedProgram(config config.Component, logger log.Component) program.CELProgram {
	var includeList, excludeList []string
	if config.GetBool("exclude_pause_container") {
		excludeList = containers.GetPauseContainerExcludeList()
	}

	return program.CELProgram{
		Name:    "ContainerPausedProgram",
		Include: createProgramFromOldFilters(includeList, filter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerType, logger),
	}
}

// ContainerSBOMProgram creates a program for filtering container SBOMs
func ContainerSBOMProgram(config config.Component, logger log.Component) program.CELProgram {
	includeList := config.GetStringSlice("sbom.container_image.container_include")
	excludeList := config.GetStringSlice("sbom.container_image.container_exclude")

	if config.GetBool("sbom.container_image.exclude_pause_container") {
		excludeList = append(excludeList, containers.GetPauseContainerExcludeList()...)
	}

	return program.CELProgram{
		Name:    "ContainerSBOMProgram",
		Include: createProgramFromOldFilters(includeList, filter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerType, logger),
	}
}
