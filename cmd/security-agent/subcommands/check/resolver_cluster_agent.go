// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package check holds check related files
package check

import (
	"context"
	"time"

	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func complianceKubernetesProvider(_ctx context.Context) (dynamic.Interface, compliance.KubernetesGroupsAndResourcesProvider, error) {
	ctx, cancel := context.WithTimeout(_ctx, 2*time.Second)
	defer cancel()
	apiCl, err := apiserver.WaitForAPIClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	return apiCl.DynamicCl, apiCl.Cl.Discovery().ServerGroupsAndResources, nil
}
