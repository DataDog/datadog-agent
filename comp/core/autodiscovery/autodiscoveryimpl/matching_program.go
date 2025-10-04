// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autodiscoveryimpl implements common structs used in the Autodiscovery code.
package autodiscoveryimpl

import (
	"errors"
	"strings"

	"github.com/google/cel-go/cel"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/celprogram"
)

const (
	containerImageField    = string(workloadfilter.ContainerType) + ".image"
	serviceNameField       = string(workloadfilter.ServiceType) + ".name"
	serviceNamespaceField  = string(workloadfilter.ServiceType) + ".namespace"
	endpointNameField      = string(workloadfilter.EndpointType) + ".name"
	endpointNamespaceField = string(workloadfilter.EndpointType) + ".namespace"
)

type matchingProgram struct {
	program cel.Program
	target  workloadfilter.ResourceType
}

func (m *matchingProgram) IsMatched(obj workloadfilter.Filterable) bool {
	if m == nil || m.program == nil {
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

func (m *matchingProgram) GetTargetType() workloadfilter.ResourceType {
	return m.target
}

// extractRuleMetadata extracts the rule list, resource type and CEL identifier from the given workloadfilter.Rules.
// This method is responsible for the priority order of the rules:
// Containers > Services > Endpoints.
func extractRuleMetadata(rules workloadfilter.Rules) (ruleList []string, objectType workloadfilter.ResourceType, celADID adtypes.CelIdentifier) {
	switch {
	case len(rules.Containers) > 0:
		return rules.Containers, workloadfilter.ContainerType, adtypes.CelContainerIdentifier
	case len(rules.KubeServices) > 0:
		return rules.KubeServices, workloadfilter.ServiceType, adtypes.CelServiceIdentifier
	case len(rules.KubeEndpoints) > 0:
		return rules.KubeEndpoints, workloadfilter.EndpointType, adtypes.CelEndpointIdentifier
	default:
		return nil, "", ""
	}
}

// checkRuleRecommendations checks if the given rules contain the recommended fields for the given CEL identifier.
func checkRuleRecommendations(rules string, celADID adtypes.CelIdentifier) error {
	switch celADID {
	case adtypes.CelContainerIdentifier:
		if !strings.Contains(rules, containerImageField) {
			return errors.New("missing recommended field: " + containerImageField)
		}
	case adtypes.CelServiceIdentifier:
		if !strings.Contains(rules, serviceNamespaceField) {
			return errors.New("missing recommended field: " + serviceNamespaceField)
		}
		rules := strings.ReplaceAll(rules, serviceNamespaceField, "")
		if !strings.Contains(rules, serviceNameField) {
			return errors.New("missing recommended field: " + serviceNameField)
		}
	case adtypes.CelEndpointIdentifier:
		if !strings.Contains(rules, endpointNamespaceField) {
			return errors.New("missing recommended field: " + endpointNamespaceField)
		}
		rules := strings.ReplaceAll(rules, endpointNamespaceField, "")
		if !strings.Contains(rules, endpointNameField) {
			return errors.New("missing recommended field: " + endpointNameField)
		}
	}
	return nil
}

// createMatchingProgram creates a MatchingProgram from the given workloadfilter.Rules.
// It returns nil if no rules are defined.
func createMatchingProgram(rules workloadfilter.Rules) (program integration.MatchingProgram, celADID adtypes.CelIdentifier, compileErr error, recError error) {
	ruleList, objectType, celADID := extractRuleMetadata(rules)
	if len(ruleList) == 0 {
		return nil, "", nil, nil
	}

	combinedRule := strings.Join(ruleList, " || ")

	celprg, compileErr := celprogram.CreateCELProgram(combinedRule, objectType)
	if compileErr != nil {
		return nil, "", compileErr, nil
	}

	recError = checkRuleRecommendations(combinedRule, celADID)

	return &matchingProgram{
		program: celprg,
		target:  objectType,
	}, celADID, nil, recError
}
