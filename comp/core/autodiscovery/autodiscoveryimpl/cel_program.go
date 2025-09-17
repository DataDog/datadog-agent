// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autodiscoveryimpl implements common structs used in the Autodiscovery code.
package autodiscoveryimpl

import (
	"strings"

	"github.com/google/cel-go/cel"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/celprogram"
)

type matchingProgram struct {
	program cel.Program
	rawRule string
	target  workloadfilter.ResourceType
	err     error
}

func (m *matchingProgram) IsMatched(obj workloadfilter.Filterable) bool {
	if m.program == nil || m.err != nil {
		return false
	}
	out, _, err := m.program.Eval(map[string]any{
		string(obj.Type()): obj.Serialize(),
	})
	if err != nil {
		return false
	}
	result, ok := out.Value().(bool)
	if !ok {
		return false
	}
	return result
}

func (m *matchingProgram) GetError() error {
	return m.err
}

func (m *matchingProgram) GetTargetType() workloadfilter.ResourceType {
	return m.target
}

// createMatchingProgram creates a MatchingProgram from the given workloadfilter.Rules.
// It returns nil if no rules are defined.
func createMatchingProgram(rules workloadfilter.Rules) (program integration.MatchingProgram, err error) {
	switch {
	case len(rules.Containers) > 0:
		return createProgram(rules.Containers, workloadfilter.ContainerType)
	case len(rules.Pods) > 0:
		return createProgram(rules.Pods, workloadfilter.PodType)
	case len(rules.KubeServices) > 0:
		return createProgram(rules.KubeServices, workloadfilter.ServiceType)
	case len(rules.KubeEndpoints) > 0:
		return createProgram(rules.KubeEndpoints, workloadfilter.EndpointType)
	default:
		return nil, nil
	}
}

func createProgram(rules []string, objectType workloadfilter.ResourceType) (program integration.MatchingProgram, err error) {
	if len(rules) == 0 {
		return nil, nil
	}

	combinedRule := strings.Join(rules, " || ")
	celprg, err := celprogram.CreateCELProgram(combinedRule, objectType)

	if err != nil {
		return nil, err
	}

	return &matchingProgram{
		program: celprg,
		rawRule: combinedRule,
		target:  objectType,
		err:     nil,
	}, nil
}
