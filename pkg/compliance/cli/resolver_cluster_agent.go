// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix && kubeapiserver

package cli

import (
	"context"
	"time"

	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func getAPIClient(ctx context.Context) (*apiserver.APIClient, error) {
	clientCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return apiserver.WaitForAPIClient(clientCtx)
}

func complianceKubernetesProvider(ctx context.Context) (dynamic.Interface, compliance.KubernetesGroupsAndResourcesProvider, error) {
	apiCl, err := getAPIClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	return apiCl.DynamicCl, apiCl.Cl.Discovery().ServerGroupsAndResources, nil
}

func startComplianceReflectorStore(ctx context.Context) *compliance.ReflectorStore {
	apiCl, err := getAPIClient(ctx)
	if err != nil {
		return nil
	}

	store := compliance.NewReflectorStore(apiCl.Cl)
	store.Run(ctx.Done())

	// Wait for the reflector to sync with a timeout
	syncCtx, syncCancel := context.WithTimeout(ctx, 30*time.Second)
	defer syncCancel()
	for !store.HasSynced() {
		select {
		case <-syncCtx.Done():
			return nil
		case <-time.After(100 * time.Millisecond):
		}
	}

	return store
}
