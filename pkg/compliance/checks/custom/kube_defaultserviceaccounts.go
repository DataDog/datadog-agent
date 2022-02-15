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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func init() {
	registerCustomCheck("kubernetesDefaultServiceAccounts", kubernetesDefaultServiceAccountsCheck)
}

func kubernetesDefaultServiceAccountsCheck(e env.Env, ruleID string, vars map[string]string, _ *eval.IterableExpression) (*compliance.Report, error) {
	if e.KubeClient() == nil {
		return nil, fmt.Errorf("unable to run kubernetesDefaultServiceAccounts check for rule: %s - Kubernetes client not initialized", ruleID)
	}

	// List all `default` service accounts
	serviceAccounts, err := e.KubeClient().Resource(schema.GroupVersionResource{
		Resource: "serviceaccounts",
		Version:  "v1",
	}).List(context.TODO(), metav1.ListOptions{
		FieldSelector: "metadata.name=default",
	})
	if err != nil {
		return nil, fmt.Errorf("error while listing serviceaccounts - rule: %s - err: %v", ruleID, err)
	}

	// No default serviceaccounts
	if len(serviceAccounts.Items) == 0 {
		return &compliance.Report{Passed: true, Aggregated: true}, nil
	}

	// Checking that all `default` service accounts have `automountServiceAccountToken` set to false
	saLookup := make(map[string]unstructured.Unstructured, len(serviceAccounts.Items))
	for _, sa := range serviceAccounts.Items {
		activated := true // default if not set
		val, found := sa.UnstructuredContent()["automountServiceAccountToken"]
		if found {
			activated, _ = val.(bool)
		}

		if activated {
			resource := compliance.NewKubeUnstructuredResource(sa)
			return compliance.BuildReportForUnstructured(false, true, resource), nil
		}

		saLookup[sa.GetNamespace()+"/"+sa.GetName()] = sa
	}

	// Checking all cluster rolebindings to verify none are assigned to any `default` service accounts
	clusterRoles, err := e.KubeClient().Resource(schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Resource: "clusterrolebindings",
		Version:  "v1",
	}).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error while listing clusterrolebindings - rule: %s - err: %v", ruleID, err)
	}

	hasRef, sa, err := hasReferences(clusterRoles, saLookup)
	if err != nil {
		return nil, fmt.Errorf("error while checking SA references in clusterrolebindings - rule: %s - err: %v", ruleID, err)
	}

	if hasRef {
		return compliance.BuildReportForUnstructured(false, true, compliance.NewKubeUnstructuredResource(*sa)), nil
	}

	roles, err := e.KubeClient().Resource(schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Resource: "rolebindings",
		Version:  "v1",
	}).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error while listing rolebindings - rule: %s - err: %v", ruleID, err)
	}

	hasRef, sa, err = hasReferences(roles, saLookup)
	if err != nil {
		return nil, fmt.Errorf("error while checking SA references in rolesbinding - rule: %s - err: %v", ruleID, err)
	}

	if hasRef {
		return compliance.BuildReportForUnstructured(false, true, compliance.NewKubeUnstructuredResource(*sa)), nil
	}

	serviceAccount := compliance.NewKubeUnstructuredResource(serviceAccounts.Items[0])
	report := compliance.BuildReportForUnstructured(true, true, serviceAccount)
	return report, nil
}

func hasReferences(roles *unstructured.UnstructuredList, saLookup map[string]unstructured.Unstructured) (bool, *unstructured.Unstructured, error) {
	// Check if a role/clusterrole has reference to default any of default SA found above
	for _, role := range roles.Items {
		subjects, found, err := unstructured.NestedSlice(role.UnstructuredContent(), "subjects")
		if err != nil {
			return false, nil, err
		}
		if !found {
			continue
		}

		for _, obj := range subjects {
			subject, ok := obj.(map[string]interface{})
			if !ok {
				return false, nil, fmt.Errorf("unable to parse role subjects")
			}

			kind, found, err := unstructured.NestedString(subject, "kind")
			if err != nil {
				return false, nil, err
			}
			if !found {
				continue
			}

			namespace, found, err := unstructured.NestedString(subject, "namespace")
			if err != nil {
				return false, nil, err
			}
			if !found {
				continue
			}

			name, found, err := unstructured.NestedString(subject, "name")
			if err != nil {
				return false, nil, err
			}
			if !found {
				continue
			}

			if kind == rbacv1.ServiceAccountKind {
				if sa, found := saLookup[namespace+"/"+name]; found {
					return true, &sa, nil
				}
			}
		}
	}

	return false, nil, nil
}
