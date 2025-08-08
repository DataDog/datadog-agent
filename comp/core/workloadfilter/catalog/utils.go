// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	legacyFilter "github.com/DataDog/datadog-agent/pkg/util/containers"
)

// createProgramFromOldFilters handles the conversion of old filters to new filters and creates a CEL program.
// Returns both the program and any errors encountered during creation.
func createProgramFromOldFilters(oldFilters []string, objectType workloadfilter.ResourceType) (cel.Program, error) {
	filterString, err := convertOldToNewFilter(oldFilters, objectType)
	if err != nil {
		return nil, err
	}

	program, err := createCELProgram(filterString, objectType)
	if err != nil {
		return nil, err
	}

	return program, nil
}

// joinValSeparator and stringOrError were copied from cel-go/ext/strings.go to
// avoid pulling in the entire library
func joinValSeparator(strs traits.Lister, separator string) (string, error) {
	sz := strs.Size().(types.Int)
	var sb strings.Builder
	for i := types.Int(0); i < sz; i++ {
		if i != 0 {
			sb.WriteString(separator)
		}
		elem := strs.Get(i)
		str, ok := elem.(types.String)
		if !ok {
			return "", fmt.Errorf("join: invalid input: %v", elem)
		}
		sb.WriteString(string(str))
	}
	return sb.String(), nil
}

func stringOrError(str string, err error) ref.Val {
	if err != nil {
		return types.NewErr(err.Error())
	}
	return types.String(str)
}

func createCELProgram(rules string, objectType workloadfilter.ResourceType) (cel.Program, error) {
	if rules == "" {
		return nil, nil
	}
	env, err := cel.NewEnv(
		cel.Types(&workloadfilter.Container{}, &workloadfilter.Pod{}, &workloadfilter.Process{}),
		cel.Variable(string(objectType), cel.ObjectType(convertTypeToProtoType(objectType))),
		// Process has cmdline as a list of strings, join() is needed to match against it as a single string.
		cel.Function("join",
			cel.MemberOverload("list_join", []*cel.Type{cel.ListType(cel.StringType)}, cel.StringType,
				cel.UnaryBinding(func(list ref.Val) ref.Val {
					l := list.(traits.Lister)
					return stringOrError(joinValSeparator(l, ""))
				}))),
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
	var processPatterns []string

	for _, oldFilter := range oldFilters {

		if oldFilter == "" {
			continue
		}

		// Legacy process denylist patterns only support raw regex
		if objectType == workloadfilter.ProcessType {
			processPatterns = append(processPatterns, oldFilter)
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

	// Combine process patterns into a single expression to avoid multiple calls to join()
	if len(processPatterns) > 0 {
		combinedPattern := strings.Join(processPatterns, "|")
		newFilters = append(newFilters, fmt.Sprintf(
			"process.cmdline.join().matches(%s)",
			strconv.Quote(combinedPattern),
		))
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
	case workloadfilter.ImageType:
		return "datadog.filter.FilterImage"
	case workloadfilter.ProcessType:
		return "datadog.filter.FilterProcess"
	default:
		return ""
	}
}
