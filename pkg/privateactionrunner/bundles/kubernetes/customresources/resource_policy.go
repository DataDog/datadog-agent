// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_customresources

import (
	"fmt"
	"strings"

	"k8s.io/apiextensions-apiserver/pkg/apihelpers"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
)

// resourcePolicy owns authorization of task-provided Kubernetes resources.
// A nil allowlist selects backward-compatible CRD mode. A non-nil allowlist,
// including an empty one, selects exact-match mode.
type resourcePolicy struct {
	allowedResources map[string]struct{}
}

func newResourcePolicy(allowedResources []string) resourcePolicy {
	if allowedResources == nil {
		return resourcePolicy{}
	}

	allowed := make(map[string]struct{}, len(allowedResources))
	for _, resource := range allowedResources {
		allowed[resource] = struct{}{}
	}
	return resourcePolicy{allowedResources: allowed}
}

func (p resourcePolicy) groupVersionResource(group, version, resource string) (schema.GroupVersionResource, error) {
	if group != "" {
		if err := validateResourceIdentifier("group", group, utilvalidation.IsDNS1123Subdomain); err != nil {
			return schema.GroupVersionResource{}, err
		}
	}
	if err := validateResourceIdentifier("version", version, utilvalidation.IsDNS1035Label); err != nil {
		return schema.GroupVersionResource{}, err
	}
	if err := validateResourceIdentifier("resource", resource, utilvalidation.IsDNS1035Label); err != nil {
		return schema.GroupVersionResource{}, err
	}

	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	key := version + "/" + resource
	if group != "" {
		key = group + "/" + key
	}
	if p.allowedResources != nil {
		if _, allowed := p.allowedResources[key]; !allowed {
			return schema.GroupVersionResource{}, fmt.Errorf("Kubernetes resource %q is not in private_action_runner.kubernetes_allowed_custom_resources", key)
		}
		return gvr, nil
	}

	if group == "" {
		return schema.GroupVersionResource{}, fmt.Errorf("core Kubernetes resource %q requires an explicit entry in private_action_runner.kubernetes_allowed_custom_resources", key)
	}
	if isNativeAPIGroup(group) {
		return schema.GroupVersionResource{}, fmt.Errorf("native Kubernetes API group %q is not available through custom resource actions", group)
	}
	return gvr, nil
}

func validateResourceIdentifier(field, value string, validateIdentifier func(string) []string) error {
	if problems := validateIdentifier(value); len(problems) > 0 {
		return fmt.Errorf("invalid Kubernetes resource %s %q: %s", field, value, strings.Join(problems, ", "))
	}
	return nil
}

func isNativeAPIGroup(group string) bool {
	return kubernetesscheme.Scheme.IsGroupRegistered(group) || apihelpers.IsProtectedCommunityGroup(group)
}
