// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package gke implements the InjectionPattern interface for GKE Gateway
package gke

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	gcpTrafficExtensionCRDName = "gcptrafficextensions.networking.gke.io"
)

var (
	gatewayGVR          = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
	trafficExtensionGVR = schema.GroupVersionResource{Group: "networking.gke.io", Version: "v1", Resource: "gcptrafficextensions"}
	crdGVR              = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
)

// Detect checks if GKE Gateway is installed in the cluster by using the dynamic client to check for the presence of the GCPTrafficExtension CRD
func Detect(ctx context.Context, client dynamic.Interface) (bool, error) {
	_, err := client.Resource(crdGVR).Get(ctx, gcpTrafficExtensionCRDName, metav1.GetOptions{})
	if err == nil || apierrors.IsNotFound(err) {
		return err == nil, nil
	}

	return false, err
}
