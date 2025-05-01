// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"

	"github.com/DataDog/datadog-agent/comp/core/config"
	common "github.com/DataDog/datadog-agent/comp/core/filter/common"
	filter "github.com/DataDog/datadog-agent/comp/core/filter/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// ContainerMetricsProgram creates a program for filtering container metrics
func ContainerMetricsProgram(config config.Component, logger log.Component) common.InclExclProgram {
	includeList := config.GetStringSlice("container_include_metrics")
	excludeList := config.GetStringSlice("container_exclude_metrics")

	return common.InclExclProgram{
		Include: createProgramFromOldFilters(includeList, filter.ContainerKey, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerKey, logger),
	}
}

// ContainerLogsProgram creates a program for filtering container logs
func ContainerLogsProgram(config config.Component, logger log.Component) common.InclExclProgram {
	includeList := config.GetStringSlice("container_include_logs")
	excludeList := config.GetStringSlice("container_exclude_logs")

	return common.InclExclProgram{
		Include: createProgramFromOldFilters(includeList, filter.ContainerKey, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerKey, logger),
	}
}

// ContainerACLegacyProgram creates a program for filtering container via legacy `AC` filters
func ContainerACLegacyProgram(config config.Component, logger log.Component) common.InclExclProgram {
	includeList := config.GetStringSlice("ac_include")
	excludeList := config.GetStringSlice("ac_exclude")

	return common.InclExclProgram{
		Include: createProgramFromOldFilters(includeList, filter.ContainerKey, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerKey, logger),
	}
}

// ContainerGlobalProgram creates a program for filtering container globally
func ContainerGlobalProgram(config config.Component, logger log.Component) common.InclExclProgram {
	includeList := config.GetStringSlice("container_include")
	excludeList := config.GetStringSlice("container_exclude")

	return common.InclExclProgram{
		Include: createProgramFromOldFilters(includeList, filter.ContainerKey, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerKey, logger),
	}
}

// ContainerADAnnotationsProgram creates a program for filtering container annotations
func ContainerADAnnotationsProgram(_ config.Component, logger log.Component) common.InclExclProgram {
	excludeFilter := `("ad.datadoghq.com/" + container.name + ".exclude") in container.annotations && container.annotations["ad.datadoghq.com/" + container.name + ".exclude"] == "true"`
	excludeProgram, err := createCELProgram(excludeFilter, filter.ContainerKey)

	if err != nil {
		logger.Warnf("Error creating CEL filtering program: %v", err)
	}

	return common.InclExclProgram{
		Exclude: excludeProgram,
	}
}

// ContainerPausedProgram creates a program for filtering paused containers
func ContainerPausedProgram(config config.Component, logger log.Component) common.InclExclProgram {
	var includeList, excludeList []string
	if config.GetBool("exclude_pause_container") {
		excludeList = containers.GetPauseContainerExcludeList()
	}

	return common.InclExclProgram{
		Include: createProgramFromOldFilters(includeList, filter.ContainerKey, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerKey, logger),
	}
}

// ContainerSBOMProgram creates a program for filtering container SBOMs
func ContainerSBOMProgram(config config.Component, logger log.Component) common.InclExclProgram {
	includeList := config.GetStringSlice("sbom.container_image.container_include")
	excludeList := config.GetStringSlice("sbom.container_image.container_exclude")

	if config.GetBool("sbom.container_image.exclude_pause_container") {
		excludeList = append(excludeList, containers.GetPauseContainerExcludeList()...)
	}

	return common.InclExclProgram{
		Include: createProgramFromOldFilters(includeList, filter.ContainerKey, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.ContainerKey, logger),
	}
}

// createProgramFromOldFilters handles the conversion of old filters to new filters and creates a CEL program.
func createProgramFromOldFilters(oldFilters []string, key filter.ResourceType, logger log.Component) cel.Program {
	filterString, err := convertOldToNewFilter(oldFilters)
	if err != nil {
		logger.Warnf("Error converting filters: %v", err)
		return nil
	}

	program, progErr := createCELProgram(filterString, key)
	if progErr != nil {
		logger.Warnf("Error creating CEL filtering program: %v", progErr)
		return nil
	}

	return program
}

func createCELProgram(rules string, key filter.ResourceType) (cel.Program, error) {
	if rules == "" {
		return nil, nil
	}
	env, err := cel.NewEnv(
		cel.Variable(string(key), cel.MapType(cel.StringType, cel.AnyType)),
	)
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(rules)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, err
	}
	return prg, nil
}

// Map to associate old filter prefixes with new filter fields
var containerFieldMapping = map[string]string{
	"id":             fmt.Sprintf("%s.id.matches", filter.ContainerKey),
	"name":           fmt.Sprintf("%s.name.matches", filter.ContainerKey),
	"image":          fmt.Sprintf("%s.image.matches", filter.ContainerKey),
	"kube_namespace": fmt.Sprintf("%s.namespace.matches", filter.ContainerKey),
}

// convertOldToNewFilter converts the legacy regex ad filter format to the google cel format.
//
// Old Format: []string{"image:nginx.*", "name:xyz-.*"},
// New Format: "container.name.matches('xyz-.*') || container.image.matches('nginx.*')""
func convertOldToNewFilter(old []string) (string, error) {
	var newFilters []string
	for _, filter := range old {

		if filter == "" {
			continue
		}

		// Split the filter into key and value using the first colon
		parts := strings.SplitN(filter, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid filter format: %s", filter)
		}

		key, value := parts[0], parts[1]
		celsafeValue := celEscape(value)
		// Map the key to the new format
		if newField, ok := containerFieldMapping[key]; ok {
			newFilters = append(newFilters, fmt.Sprintf("%s('%s')", newField, celsafeValue))
		} else {
			return "", fmt.Errorf("unsupported filter key: %s", key)
		}
	}
	return strings.Join(newFilters, " || "), nil
}

// celEscape escapes backslashes and double quotes for CEL compatibility
func celEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
