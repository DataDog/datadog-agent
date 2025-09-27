// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autodiscoveryimpl implements common structs used in the Autodiscovery code.
package autodiscoveryimpl

import (
	"strings"

	"github.com/google/cel-go/cel"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/celprogram"
)

type matchingProgram struct {
	program cel.Program
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

// extractRuleMetadata extracts the rule list, resource type and CEL identifier from the given workloadfilter.Rules.
// This method is responsible for the priority order of the rules:
// Containers > Pods > Services > Endpoints.
func extractRuleMetadata(rules workloadfilter.Rules) (ruleList []string, objectType workloadfilter.ResourceType, celADID adtypes.CelIdentifier) {
	switch {
	case len(rules.Containers) > 0:
		return rules.Containers, workloadfilter.ContainerType, adtypes.CelContainerIdentifier
	case len(rules.Pods) > 0:
		return rules.Pods, workloadfilter.PodType, adtypes.CelPodIdentifier
	case len(rules.KubeServices) > 0:
		return rules.KubeServices, workloadfilter.ServiceType, adtypes.CelServiceIdentifier
	case len(rules.KubeEndpoints) > 0:
		return rules.KubeEndpoints, workloadfilter.EndpointType, adtypes.CelEndpointIdentifier
	default:
		return nil, "", ""
	}
}

// createMatchingProgram creates a MatchingProgram from the given workloadfilter.Rules.
// It returns nil if no rules are defined.
func createMatchingProgram(rules workloadfilter.Rules) (program integration.MatchingProgram, err error) {
	ruleList, objectType, _ := extractRuleMetadata(rules)
	if len(ruleList) == 0 {
		return nil, nil
	}

	celprg, err := celprogram.CreateCELProgram(strings.Join(ruleList, " || "), objectType)
	if err != nil {
		return nil, err
	}

	return &matchingProgram{
		program: celprg,
		target:  objectType,
		err:     nil,
	}, nil
}
