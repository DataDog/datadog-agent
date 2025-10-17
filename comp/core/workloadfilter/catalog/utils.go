// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	legacyFilter "github.com/DataDog/datadog-agent/pkg/util/containers"
)

func createFromOldFilters(name string, oldInclude, oldExclude []string, objectType workloadfilter.ResourceType, logger log.Component) program.FilterProgram {
	var initErrors []error

	includeProgram, includeErr := createProgramFromOldFilters(oldInclude, objectType)
	if includeErr != nil {
		initErrors = append(initErrors, includeErr)
		logger.Warnf("error creating include program for %s: %v", name, includeErr)
	}

	excludeProgram, excludeErr := createProgramFromOldFilters(oldExclude, objectType)
	if excludeErr != nil {
		initErrors = append(initErrors, excludeErr)
		logger.Warnf("error creating exclude program for %s: %v", name, excludeErr)
	}

	return program.CELProgram{
		Name:                 name,
		Include:              includeProgram,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}

// createProgramFromOldFilters handles the conversion of old filters to new filters and creates a CEL program.
// Returns both the program and any errors encountered during creation.
func createProgramFromOldFilters(oldFilters []string, objectType workloadfilter.ResourceType) (cel.Program, error) {
	filterString, err := convertOldToNewFilter(oldFilters, objectType)
	if err != nil {
		return nil, err
	}

	program, err := compileCELProgram(filterString, objectType)
	if err != nil {
		return nil, err
	}

	return program, nil
}

func compileCELProgram(rules string, objectType workloadfilter.ResourceType) (cel.Program, error) {
	if rules == "" {
		return nil, nil
	}
	env, err := cel.NewEnv(
		cel.Types(&workloadfilter.Container{}, &workloadfilter.Pod{}, &workloadfilter.Process{}),
		cel.Variable(string(objectType), cel.ObjectType(convertTypeToProtoType(objectType))),
	)
	if err != nil {
		return nil, err
	}
	abstractSyntaxTree, issues := env.Compile(rules)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	prg, err := env.Program(abstractSyntaxTree, cel.EvalOptions(cel.OptOptimize))
	if err != nil {
		return nil, err
	}
	return prg, nil
}

// getFieldMapping creates a map to associate old filter prefixes with new filter fields
func getFieldMapping(objectType workloadfilter.ResourceType) map[string]string {
	return map[string]string{
		"name":  fmt.Sprintf("%s.name.matches", objectType),
		"image": fmt.Sprintf("%s.image.matches", objectType),
		"kube_namespace": func() string {
			if objectType == workloadfilter.ContainerType {
				return fmt.Sprintf("%s.%s.namespace.matches", objectType, workloadfilter.PodType)
			}
			return fmt.Sprintf("%s.namespace.matches", objectType)

		}(),
	}
}

// convertOldToNewFilter converts the legacy regex ad filter format to cel-go format.
//
// Old Format: []string{"image:nginx.*", "name:xyz-.*"},
// New Format: "container.name.matches('xyz-.*') || container.image.matches('nginx.*')"
func convertOldToNewFilter(oldFilters []string, objectType workloadfilter.ResourceType) (string, error) {
	if oldFilters == nil {
		return "", nil
	}

	legacyFieldMapping := getFieldMapping(objectType)

	var newFilters []string
	for _, oldFilter := range oldFilters {

		if oldFilter == "" {
			continue
		}

		// Split the filter into key and value using the first colon
		key, value, ok := strings.Cut(oldFilter, ":")
		if !ok {
			return "", fmt.Errorf("invalid filter format: %s", oldFilter)
		}

		// Check if the key applies for the particular workload type
		if objectType != workloadfilter.ContainerType && key == "image" {
			continue
		}
		if objectType == workloadfilter.PodType && key != "kube_namespace" {
			continue
		}

		// Legacy support for image filtering
		if key == "image" {
			value = legacyFilter.PreprocessImageFilter(value)
		}

		if newField, ok := legacyFieldMapping[key]; ok {
			newFilters = append(newFilters, fmt.Sprintf(`%s(%s)`, newField, strconv.Quote(value)))
		} else {
			return "", fmt.Errorf("container filter %s:%s is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'", key, value)
		}
	}
	return strings.Join(newFilters, " || "), nil
}

// convertTypeToProtoType converts a filter.ResourceType to its corresponding proto type string.
func convertTypeToProtoType(key workloadfilter.ResourceType) string {
	switch key {
	case workloadfilter.ContainerType:
		return "datadog.filter.FilterContainer"
	case workloadfilter.PodType:
		return "datadog.filter.FilterPod"
	case workloadfilter.ServiceType:
		return "datadog.filter.FilterKubeService"
	case workloadfilter.EndpointType:
		return "datadog.filter.FilterKubeEndpoint"
	case workloadfilter.ProcessType:
		return "datadog.filter.FilterProcess"
	default:
		return ""
	}
}

func createCELExcludeProgram(name string, rules string, objectType workloadfilter.ResourceType, logger log.Component) program.FilterProgram {
	excludeProgram, excludeErr := compileCELProgram(rules, objectType)
	if excludeErr != nil {
		logger.Criticalf(`failed to compile '%s' from 'cel_workload_exclude' filters: %v`, name, excludeErr)
		logger.Flush()
		os.Exit(1)
	}
	return program.CELProgram{
		Name:                 name,
		Exclude:              excludeProgram,
		InitializationErrors: nil,
	}
}
