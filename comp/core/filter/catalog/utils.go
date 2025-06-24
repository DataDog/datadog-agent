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

	filter "github.com/DataDog/datadog-agent/comp/core/filter/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	legacyFilter "github.com/DataDog/datadog-agent/pkg/util/containers"
)

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
		cel.Types(&filter.Container{}, &filter.Pod{}),
		cel.Variable(string(key), cel.ObjectType(convertTypeToProtoType(key))),
	)
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(rules)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	prg, err := env.Program(ast, cel.EvalOptions(cel.OptOptimize))
	if err != nil {
		return nil, err
	}
	return prg, nil
}

// Map to associate old filter prefixes with new filter fields
var containerFieldMapping = map[string]string{
	"id":             fmt.Sprintf("%s.id.matches", filter.ContainerType),
	"name":           fmt.Sprintf("%s.name.matches", filter.ContainerType),
	"image":          fmt.Sprintf("%s.image.matches", filter.ContainerType),
	"kube_namespace": fmt.Sprintf("%s.%s.namespace.matches", filter.ContainerType, filter.PodType),
}

// getValidKeys returns a slice of valid keys for legacy container filters.
func getValidKeys() []string {
	keys := make([]string, 0, len(containerFieldMapping))
	for key := range containerFieldMapping {
		keys = append(keys, key)
	}
	return keys
}

// convertOldToNewFilter converts the legacy regex ad filter format to cel-go format.
//
// Old Format: []string{"image:nginx.*", "name:xyz-.*"},
// New Format: "container.name.matches('xyz-.*') || container.image.matches('nginx.*')"
func convertOldToNewFilter(old []string) (string, error) {
	var newFilters []string
	for _, filter := range old {

		if filter == "" {
			continue
		}

		// Split the filter into key and value using the first colon
		key, value, ok := strings.Cut(filter, ":")
		if !ok {
			return "", fmt.Errorf("invalid filter format: %s", filter)
		}

		celsafeValue := celEscape(value)

		// Legacy support for image filtering
		if key == "image" {
			celsafeValue = legacyFilter.PreprocessImageFilter(celsafeValue)
		}

		if newField, ok := containerFieldMapping[key]; ok {
			newFilters = append(newFilters, fmt.Sprintf("%s('%s')", newField, celsafeValue))
		} else {
			return "", fmt.Errorf("unsupported filter key '%s' must be in %v", key, getValidKeys())
		}
	}
	return strings.Join(newFilters, " || "), nil
}

// celEscape escapes backslashes and single quotes for CEL compatibility
func celEscape(s string) string {

	// Backslashes must be escaped because CEL parses string literals first.
	s = strings.ReplaceAll(s, `\`, `\\`)

	// Must escape the single quote because we wrap the
	// entire input within single quotes in the CEL expression.
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

// convertTypeToProtoType converts a filter.ResourceType to its corresponding proto type string.
func convertTypeToProtoType(key filter.ResourceType) string {
	switch key {
	case filter.ContainerType:
		return "datadog.filter.FilterContainer"
	case filter.PodType:
		return "datadog.filter.FilterPod"
	default:
		return ""
	}
}
