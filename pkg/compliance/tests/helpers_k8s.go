// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tests

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"k8s.io/client-go/dynamic"
)

func providerFromK8sClient(kubeclient dynamic.Interface) compliance.KubernetesProvider {
	return func(context.Context) (dynamic.Interface, compliance.KubernetesGroupsAndResourcesProvider, error) {
		return kubeclient, nil, nil
	}
}
