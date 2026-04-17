// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package nginx implements the InjectionPattern interface for ingress-nginx
package nginx

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const ingressNginxControllerName = "k8s.io/ingress-nginx"

var ingressClassGVR = schema.GroupVersionResource{
	Group:    "networking.k8s.io",
	Version:  "v1",
	Resource: "ingressclasses",
}

// Detect checks if ingress-nginx is installed by looking for an IngressClass
// with spec.controller == "k8s.io/ingress-nginx"
func Detect(ctx context.Context, client dynamic.Interface) (bool, error) {
	list, err := client.Resource(ingressClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list IngressClasses for ingress-nginx detection: %w", err)
	}

	for _, item := range list.Items {
		controllerName, found, err := unstructured.NestedString(
			item.UnstructuredContent(), "spec", "controller",
		)
		if err != nil || !found {
			continue
		}
		if controllerName == ingressNginxControllerName {
			return true, nil
		}
	}

	return false, nil
}
