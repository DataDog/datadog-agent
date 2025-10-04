// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"fmt"
	"strconv"
	"strings"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/google/cel-go/cel"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/celprogram"
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

	program, err := celprogram.CreateCELProgram(filterString, objectType)
	if err != nil {
		return nil, err
	}

	return program, nil
}

// getFieldMapping creates a map to associate old filter prefixes with new filter fields
func getFieldMapping(objectType workloadfilter.ResourceType) map[string]string {
	if objectType == workloadfilter.ImageType {
		// only support "image" which is the image name
		return map[string]string{
			"image": fmt.Sprintf("%s.name.matches", objectType),
		}
	}
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
		if objectType != workloadfilter.ContainerType && objectType != workloadfilter.ImageType && key == "image" {
			continue
		}
		if objectType == workloadfilter.ImageType && key != "image" {
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
