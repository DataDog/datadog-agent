// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

package integration

import (
	"errors"
	"strings"

	"github.com/google/cel-go/cel"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/celprogram"
)

const (
	containerImageField    = string(workloadfilter.ContainerType) + ".image"
	processCmdlineField    = string(workloadfilter.ProcessType) + ".cmdline"
	serviceNameField       = string(workloadfilter.KubeServiceType) + ".name"
	serviceNamespaceField  = string(workloadfilter.KubeServiceType) + ".namespace"
	endpointNameField      = string(workloadfilter.KubeEndpointType) + ".name"
	endpointNamespaceField = string(workloadfilter.KubeEndpointType) + ".namespace"
)

// CELMatchingProgram wraps a CEL program to implement the MatchingProgram interface
type CELMatchingProgram struct {
	program cel.Program
	target  workloadfilter.ResourceType
}

// IsMatched evaluates the CEL program against the given object
func (m *CELMatchingProgram) IsMatched(obj workloadfilter.Filterable) bool {
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
	return ok && result
}

// GetTargetType returns the target resource type of the program
func (m *CELMatchingProgram) GetTargetType() workloadfilter.ResourceType {
	return m.target
}

// ruleMetadata holds the rule list, resource type and CEL identifier for a single resource type.
type ruleMetadata struct {
	ruleList   []string
	objectType workloadfilter.ResourceType
	celADID    adtypes.CelIdentifier
}

// extractAllRuleMetadata extracts metadata for ALL non-empty rule categories from the given workloadfilter.Rules.
func extractAllRuleMetadata(rules workloadfilter.Rules) []ruleMetadata {
	var result []ruleMetadata
	if len(rules.Containers) > 0 {
		result = append(result, ruleMetadata{rules.Containers, workloadfilter.ContainerType, adtypes.CelContainerIdentifier})
	}
	if len(rules.Processes) > 0 {
		result = append(result, ruleMetadata{rules.Processes, workloadfilter.ProcessType, adtypes.CelProcessIdentifier})
	}
	if len(rules.KubeServices) > 0 {
		result = append(result, ruleMetadata{rules.KubeServices, workloadfilter.KubeServiceType, adtypes.CelServiceIdentifier})
	}
	if len(rules.KubeEndpoints) > 0 {
		result = append(result, ruleMetadata{rules.KubeEndpoints, workloadfilter.KubeEndpointType, adtypes.CelEndpointIdentifier})
	}
	return result
}

// checkRuleRecommendations checks if the given rules contain the recommended fields for the given CEL identifier.
func checkRuleRecommendations(rules string, celADID adtypes.CelIdentifier) error {
	switch celADID {
	case adtypes.CelContainerIdentifier:
		if !strings.Contains(rules, containerImageField) {
			return errors.New("missing recommended field: " + containerImageField)
		}
	case adtypes.CelProcessIdentifier:
		if !strings.Contains(rules, processCmdlineField) {
			return errors.New("missing recommended field: " + processCmdlineField)
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

// CreateMatchingPrograms creates MatchingPrograms for all resource types that have rules defined.
// It returns a map of programs keyed by resource type and the list of CEL AD identifiers.
// If checkRecommendations is true and any type fails recommendation checks, the call fails.
func CreateMatchingPrograms(rules workloadfilter.Rules, checkRecommendations bool) (map[workloadfilter.ResourceType]MatchingProgram, []adtypes.CelIdentifier, error) {
	allMeta := extractAllRuleMetadata(rules)
	if len(allMeta) == 0 {
		return nil, nil, nil
	}

	programs := make(map[workloadfilter.ResourceType]MatchingProgram, len(allMeta))
	var celADIDs []adtypes.CelIdentifier

	for _, meta := range allMeta {
		combinedRule := strings.Join(meta.ruleList, " || ")

		celprg, err := celprogram.CreateCELProgram(combinedRule, meta.objectType)
		if err != nil {
			return nil, nil, err
		}

		if checkRecommendations {
			if err := checkRuleRecommendations(combinedRule, meta.celADID); err != nil {
				return nil, nil, err
			}
		}

		programs[meta.objectType] = &CELMatchingProgram{
			program: celprg,
			target:  meta.objectType,
		}
		celADIDs = append(celADIDs, meta.celADID)
	}

	return programs, celADIDs, nil
}
