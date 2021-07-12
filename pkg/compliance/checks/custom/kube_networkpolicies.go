// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package custom

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func init() {
	registerCustomCheck("kubernetesNetworkPolicies", kubernetesNetworkPoliciesCheck)
}

func kubernetesNetworkPoliciesCheck(e env.Env, ruleID string, vars map[string]string, _ *eval.IterableExpression) (*compliance.Report, error) {
	if e.KubeClient() == nil {
		return nil, fmt.Errorf("unable to run kubernetesNetworkPolicies check for rule: %s - Kubernetes client not initialized", ruleID)
	}

	// Build namespace lookup
	namespaces, err := e.KubeClient().Resource(schema.GroupVersionResource{
		Resource: "namespaces",
		Version:  "v1",
	}).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error while listing namespaces - rule: %s - err: %v", ruleID, err)
	}
	if len(namespaces.Items) == 0 {
		return nil, fmt.Errorf("error while listing namespaces - none found - rule: %s", ruleID)
	}

	nsLookup := make(map[string]struct{}, len(namespaces.Items))
	for _, ns := range namespaces.Items {
		nsLookup[ns.GetName()] = struct{}{}
	}

	policies, err := e.KubeClient().Resource(schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Resource: "networkpolicies",
		Version:  "v1",
	}).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error while listing network policies - rule: %s - err: %v", ruleID, err)
	}

	for _, policy := range policies.Items {
		delete(nsLookup, policy.GetNamespace())
	}

	report := compliance.Report{
		Data: event.Data{
			compliance.KubeResourceFieldKind:    namespaces.Items[0].GroupVersionKind().Kind,
			compliance.KubeResourceFieldGroup:   namespaces.Items[0].GroupVersionKind().Group,
			compliance.KubeResourceFieldVersion: namespaces.Items[0].GroupVersionKind().Version,
		},
		Aggregated: true,
	}

	if len(nsLookup) > 0 {
		var failingNs string
		for ns := range nsLookup {
			failingNs = ns
			break
		}

		report.Passed = false
		report.Data[compliance.KubeResourceFieldName] = failingNs
	} else {
		report.Passed = true
		report.Data[compliance.KubeResourceFieldName] = namespaces.Items[0].GetName()
	}

	return &report, nil
}
