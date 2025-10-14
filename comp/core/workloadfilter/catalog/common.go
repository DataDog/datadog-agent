// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

// This file contains filter programs that can be shared across different entity types.

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// AutodiscoveryAnnotations creates a CEL program for autodiscovery annotations.
func AutodiscoveryAnnotations() program.FilterProgram {
	return program.AnnotationsProgram{
		Name:          "AutodiscoveryAnnotation",
		ExcludePrefix: "",
	}
}

// AutodiscoveryMetricsAnnotations creates a CEL program for autodiscovery metrics annotations.
func AutodiscoveryMetricsAnnotations() program.FilterProgram {
	return program.AnnotationsProgram{
		Name:          "AutodiscoveryMetricsAnnotations",
		ExcludePrefix: "metrics_",
	}
}

// AutodiscoveryLogsAnnotations creates a CEL program for autodiscovery logs annotations.
func AutodiscoveryLogsAnnotations() program.FilterProgram {
	return program.AnnotationsProgram{
		Name:          "AutodiscoveryLogsAnnotations",
		ExcludePrefix: "logs_",
	}
}

// LegacyContainerGlobalProgram creates a legacy filter program for global containerized filtering
func LegacyContainerGlobalProgram(cfg *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerGlobalProgram"
	include := cfg.ContainerInclude
	exclude := cfg.ContainerExclude

	var initErrors []error
	filter, err := containers.NewFilter(containers.GlobalFilter, include, exclude)
	if err != nil {
		initErrors = append(initErrors, err)
		logger.Warnf("Failed to create filter '%s': %v", programName, err)
	}

	return program.LegacyFilterProgram{
		Name:                 programName,
		Filter:               filter,
		InitializationErrors: initErrors,
	}
}

// LegacyContainerMetricsProgram creates a legacy filter program for containerized metrics filtering
func LegacyContainerMetricsProgram(cfg *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerMetricsProgram"
	include := cfg.ContainerIncludeMetrics
	exclude := cfg.ContainerExcludeMetrics

	var initErrors []error
	filter, err := containers.NewFilter(containers.GlobalFilter, include, exclude)
	if err != nil {
		initErrors = append(initErrors, err)
		logger.Warnf("Failed to create filter '%s': %v", programName, err)
	}

	return program.LegacyFilterProgram{
		Name:                 programName,
		Filter:               filter,
		InitializationErrors: initErrors,
	}
}

// LegacyContainerLogsProgram creates a legacy filter program for containerized logs filtering
func LegacyContainerLogsProgram(cfg *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerLogsProgram"
	include := cfg.ContainerIncludeLogs
	exclude := cfg.ContainerExcludeLogs

	var initErrors []error
	filter, err := containers.NewFilter(containers.GlobalFilter, include, exclude)
	if err != nil {
		initErrors = append(initErrors, err)
		logger.Warnf("Failed to create filter '%s': %v", programName, err)
	}

	return program.LegacyFilterProgram{
		Name:                 programName,
		Filter:               filter,
		InitializationErrors: initErrors,
	}
}

// LegacyContainerACExcludeProgram creates a legacy filter program for containerized AC exclusion filtering
func LegacyContainerACExcludeProgram(cfg *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerACExcludeProgram"
	exclude := cfg.ACExclude

	var initErrors []error
	filter, err := containers.NewFilter(containers.GlobalFilter, nil, exclude)
	if err != nil {
		initErrors = append(initErrors, err)
		logger.Warnf("Failed to create filter '%s': %v", programName, err)
	}

	return program.LegacyFilterProgram{
		Name:                 programName,
		Filter:               filter,
		InitializationErrors: initErrors,
	}
}

// LegacyContainerACIncludeProgram creates a legacy filter program for containerized AC inclusion filtering
func LegacyContainerACIncludeProgram(cfg *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerACIncludeProgram"
	include := cfg.ACInclude

	var initErrors []error
	filter, err := containers.NewFilter(containers.GlobalFilter, include, nil)
	if err != nil {
		initErrors = append(initErrors, err)
		logger.Warnf("Failed to create filter '%s': %v", programName, err)
	}

	return program.LegacyFilterProgram{
		Name:                 programName,
		Filter:               filter,
		InitializationErrors: initErrors,
	}
}

// LegacyContainerSBOMProgram creates a legacy filter program for containerized SBOM filtering
func LegacyContainerSBOMProgram(cfg *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerSBOMProgram"
	include := cfg.SBOMContainerInclude
	exclude := cfg.SBOMContainerExclude

	var initErrors []error
	filter, err := containers.NewFilter(containers.GlobalFilter, include, exclude)
	if err != nil {
		initErrors = append(initErrors, err)
		logger.Warnf("Failed to create filter '%s': %v", programName, err)
	}

	return program.LegacyFilterProgram{
		Name:                 programName,
		Filter:               filter,
		InitializationErrors: initErrors,
	}
}
