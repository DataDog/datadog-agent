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

// Detect checks whether Envoy Gateway is installed in the cluster by looking for the EnvoyPatchPolicy CRD
func Detect(ctx context.Context, client dynamic.Interface) (bool, error) {
	_, err := client.Resource(crdGVR).Get(ctx, envoyPatchPolicyCRDName, metav1.GetOptions{})
	if err == nil || errors.IsNotFound(err) {
		return err == nil, nil
	}

	return false, err
}
