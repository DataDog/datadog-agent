// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	config "github.com/DataDog/datadog-agent/comp/core/config/def"
	filter "github.com/DataDog/datadog-agent/comp/core/filter/def"
	"github.com/DataDog/datadog-agent/comp/core/filter/program"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// LegacyContainerMetricsProgram creates a program for filtering container metrics
func LegacyContainerMetricsProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyContainerMetricsProgram",
		Include: createProgramFromOldFilters(config.GetStringSlice("container_include_metrics"), filter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(config.GetStringSlice("container_exclude_metrics"), filter.ContainerType, logger),
	}
}

// LegacyContainerLogsProgram creates a program for filtering container logs
func LegacyContainerLogsProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyContainerLogsProgram",
		Include: createProgramFromOldFilters(config.GetStringSlice("container_include_logs"), filter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(config.GetStringSlice("container_exclude_logs"), filter.ContainerType, logger),
	}
}

// LegacyContainerACExcludeProgram creates a program for excluding containers via legacy `AC` filters
func LegacyContainerACExcludeProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyContainerACExcludeProgram",
		Exclude: createProgramFromOldFilters(config.GetStringSlice("ac_exclude"), filter.ContainerType, logger),
	}
}

// LegacyContainerACIncludeProgram creates a program for including containers via legacy `AC` filters
func LegacyContainerACIncludeProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyContainerACIncludeProgram",
		Include: createProgramFromOldFilters(config.GetStringSlice("ac_include"), filter.ContainerType, logger),
	}
}

// LegacyContainerGlobalProgram creates a program for filtering container globally
func LegacyContainerGlobalProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyContainerGlobalProgram",
		Include: createProgramFromOldFilters(config.GetStringSlice("container_include"), filter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(config.GetStringSlice("container_exclude"), filter.ContainerType, logger),
	}
}

// LegacyContainerSBOMProgram creates a program for filtering container SBOMs
func LegacyContainerSBOMProgram(config config.Component, logger log.Component) program.CELProgram {
	excludeList := config.GetStringSlice("sbom.container_image.container_exclude")
	if config.GetBool("sbom.container_image.exclude_pause_container") {
		excludeList = append(excludeList, containers.GetPauseContainerExcludeList()...)
	}

	return program.CELProgram{
		Name:    "LegacyContainerSBOMProgram",
		Include: createProgramFromOldFilters(config.GetStringSlice("sbom.container_image.container_include"), filter.ContainerType, logger),
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
	var excludeList []string
	if config.GetBool("exclude_pause_container") {
		excludeList = containers.GetPauseContainerExcludeList()
	}

	return program.CELProgram{
		Name:    "ContainerPausedProgram",
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerType, logger),
	}
}
