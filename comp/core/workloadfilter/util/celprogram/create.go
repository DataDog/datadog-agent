// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package celprogram contains helper functions to create CEL programs for filtering.
package celprogram

import (
	"github.com/google/cel-go/cel"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

// getEnv returns a reusable CEL environment for the given object type, building it once and storing
// it in the global in-memory cache (no expiration) so it is shared by every program.
func getEnv(objectType workloadfilter.ResourceType) (*cel.Env, error) {
	key := cache.BuildAgentKey("workloadfilter", "celprogram", "env", string(objectType))
	return cache.Get[*cel.Env](key, func() (*cel.Env, error) {
		return cel.NewEnv(
			cel.Types(&workloadfilter.Container{}, &workloadfilter.Pod{}, &workloadfilter.KubeService{}, &workloadfilter.KubeEndpoint{}, &workloadfilter.Process{}),
			cel.Variable(string(objectType), cel.ObjectType(convertTypeToProtoType(objectType))),
		)
	})
}

// CreateCELProgram creates a CEL program from the given rules and object type.
func CreateCELProgram(rules string, objectType workloadfilter.ResourceType) (cel.Program, error) {
	if rules == "" {
		return nil, nil
	}
	env, err := getEnv(objectType)
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

// convertTypeToProtoType converts a filter.ResourceType to its corresponding proto type string.
func convertTypeToProtoType(key workloadfilter.ResourceType) string {
	switch key {
	case workloadfilter.ContainerType:
		return "datadog.workloadfilter.FilterContainer"
	case workloadfilter.PodType:
		return "datadog.workloadfilter.FilterPod"
	case workloadfilter.KubeServiceType:
		return "datadog.workloadfilter.FilterKubeService"
	case workloadfilter.KubeEndpointType:
		return "datadog.workloadfilter.FilterKubeEndpoint"
	case workloadfilter.ProcessType:
		return "datadog.workloadfilter.FilterProcess"
	default:
		return ""
	}
}
