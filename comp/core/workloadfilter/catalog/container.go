// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// LegacyContainerMetricsProgram creates a program for filtering container metrics
func LegacyContainerMetricsProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyContainerMetricsProgram",
		Include: createProgramFromOldFilters(config.GetStringSlice("container_include_metrics"), workloadfilter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(config.GetStringSlice("container_exclude_metrics"), workloadfilter.ContainerType, logger),
	}
}

// LegacyContainerLogsProgram creates a program for filtering container logs
func LegacyContainerLogsProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyContainerLogsProgram",
		Include: createProgramFromOldFilters(config.GetStringSlice("container_include_logs"), workloadfilter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(config.GetStringSlice("container_exclude_logs"), workloadfilter.ContainerType, logger),
	}
}

// LegacyContainerACExcludeProgram creates a program for excluding containers via legacy `AC` filters
func LegacyContainerACExcludeProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyContainerACExcludeProgram",
		Exclude: createProgramFromOldFilters(config.GetStringSlice("ac_exclude"), workloadfilter.ContainerType, logger),
	}
}

// LegacyContainerACIncludeProgram creates a program for including containers via legacy `AC` filters
func LegacyContainerACIncludeProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyContainerACIncludeProgram",
		Include: createProgramFromOldFilters(config.GetStringSlice("ac_include"), workloadfilter.ContainerType, logger),
	}
}

// LegacyContainerGlobalProgram creates a program for filtering container globally
func LegacyContainerGlobalProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyContainerGlobalProgram",
		Include: createProgramFromOldFilters(config.GetStringSlice("container_include"), workloadfilter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(config.GetStringSlice("container_exclude"), workloadfilter.ContainerType, logger),
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
		Include: createProgramFromOldFilters(config.GetStringSlice("sbom.container_image.container_include"), workloadfilter.ContainerType, logger),
		Exclude: createProgramFromOldFilters(excludeList, workloadfilter.ContainerType, logger),
	}
}

// createContainerADAnnotationsProgram creates a program for filtering
// container annotations based on the annotation key.
func createContainerADAnnotationsProgram(programName, annotationKey string, logger log.Component) program.CELProgram {
	// Use 'in' operator to safely check if annotation exists before accessing it
	excludeFilter := fmt.Sprintf(`
		(("ad.datadoghq.com/" + container.name + ".%s") in container.pod.annotations && 
		 container.pod.annotations["ad.datadoghq.com/" + container.name + ".%s"] == "true") ||
		(("ad.datadoghq.com/%s") in container.pod.annotations && 
		 container.pod.annotations["ad.datadoghq.com/%s"] == "true")
	`, annotationKey, annotationKey, annotationKey, annotationKey)

	excludeProgram, err := createCELProgram(excludeFilter, workloadfilter.ContainerType)

	if err != nil {
		logger.Warnf("Error creating CEL filtering program: %v", err)
	}

	return program.CELProgram{
		Name:    programName,
		Exclude: excludeProgram,
	}
}

// ContainerADAnnotationsProgram creates a program for filtering container annotations
func ContainerADAnnotationsProgram(_ config.Component, logger log.Component) program.CELProgram {
	return createContainerADAnnotationsProgram("ContainerADAnnotationsProgram", "exclude", logger)
}

// ContainerADAnnotationsMetricsProgram creates a program for filtering container annotations for metrics
func ContainerADAnnotationsMetricsProgram(_ config.Component, logger log.Component) program.CELProgram {
	return createContainerADAnnotationsProgram("ContainerADAnnotationsMetricsProgram", "metrics_exclude", logger)
}

// ContainerADAnnotationsLogsProgram creates a program for filtering container annotations for logs
func ContainerADAnnotationsLogsProgram(_ config.Component, logger log.Component) program.CELProgram {
	return createContainerADAnnotationsProgram("ContainerADAnnotationsLogsProgram", "logs_exclude", logger)
}

// ContainerPausedProgram creates a program for filtering paused containers
func ContainerPausedProgram(config config.Component, logger log.Component) program.CELProgram {
	var excludeList []string
	if config.GetBool("exclude_pause_container") {
		excludeList = containers.GetPauseContainerExcludeList()
	}

	return program.CELProgram{
		Name:    "ContainerPausedProgram",
		Exclude: createProgramFromOldFilters(excludeList, workloadfilter.ContainerType, logger),
	}
}
