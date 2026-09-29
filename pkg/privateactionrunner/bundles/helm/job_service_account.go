// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package com_datadoghq_helm

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

const (
	// fullnameLabel is the standard Helm chart label ("datadog.pod-template-labels")
	// carrying the release's resolved fullname. The chart sets it to the same
	// value used to name every release-scoped resource, and — unlike the
	// cluster agent's own ServiceAccount name — it isn't affected by a custom
	// clusterAgent.rbac.serviceAccountName override, so it's a reliable way to
	// recover the fullname from a live cluster without any chart-side changes.
	fullnameLabel = "app.kubernetes.io/name"
	// jobServiceAccountSuffix is the fixed suffix the chart appends to the
	// release fullname for the helm-actions Job's ServiceAccount (see
	// cluster-agent-helm-actions-rbac.yaml in the helm-charts repo); it is not
	// configurable, so it's safe to hardcode here.
	jobServiceAccountSuffix = "-helm-actions"
)

// jobServiceAccountName derives the name of the ServiceAccount the rollback
// Job must run as: "<fullname>-helm-actions". The release fullname isn't
// otherwise recoverable in Go (it depends on Helm's name/fullnameOverride
// logic), so it's read off the cluster agent's own pod, which the chart always
// labels with its resolved fullname.
func jobServiceAccountName(ctx context.Context, client kubernetes.Interface, ownNamespace string) (string, error) {
	podName, err := common.GetSelfPodName()
	if err != nil {
		return "", fmt.Errorf("get self pod name: %w", err)
	}

	pod, err := client.CoreV1().Pods(ownNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get self pod %s/%s: %w", ownNamespace, podName, err)
	}

	fullname := pod.Labels[fullnameLabel]
	if fullname == "" {
		return "", fmt.Errorf("pod %s/%s has no %q label", ownNamespace, podName, fullnameLabel)
	}

	return fullname + jobServiceAccountSuffix, nil
}
