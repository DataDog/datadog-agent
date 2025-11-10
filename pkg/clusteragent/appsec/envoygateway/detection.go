// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package envoygateway

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// Detect whenever Envoy Gateway is installed in the cluster by using the dynamic client to check for the presence of the EnvoyExtensionPolicy CRD
func Detect(ctx context.Context, client dynamic.Interface) (bool, error) {
	_, err := client.Resource(crdGVR).Get(ctx, envoyExtensionPolicyCRDName, metav1.GetOptions{})
	if err == nil || errors.IsNotFound(err) {
		return err == nil, nil
	}

	return false, err
}
