// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package transformers

import (
	model "github.com/DataDog/agent-payload/v5/process"
	rbacv1 "k8s.io/api/rbac/v1"
)

// ExtractK8sClusterRole returns the protobuf model corresponding to a
// Kubernetes ClusterRole resource.
func ExtractK8sClusterRole(cr *rbacv1.ClusterRole) *model.ClusterRole {
	clusterRole := &model.ClusterRole{
		Metadata: extractMetadata(&cr.ObjectMeta),
		Rules:    extractPolicyRules(cr.Rules),
	}
	if cr.AggregationRule != nil {
		for _, rule := range cr.AggregationRule.ClusterRoleSelectors {
			clusterRole.AggregationRules = append(clusterRole.AggregationRules, extractLabelSelector(&rule)...)
		}
	}
	return clusterRole
}

func extractPolicyRules(r []rbacv1.PolicyRule) []*model.PolicyRule {
	rules := make([]*model.PolicyRule, 0, len(r))
	for _, rule := range r {
		rules = append(rules, &model.PolicyRule{
			ApiGroups:       rule.APIGroups,
			NonResourceURLs: rule.NonResourceURLs,
			Resources:       rule.Resources,
			ResourceNames:   rule.ResourceNames,
			Verbs:           rule.Verbs,
		})
	}
	return rules
}
