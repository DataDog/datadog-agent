// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package com_datadoghq_helm

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

const (
	// ownServiceAccountSuffix is the suffix the Helm chart appends to the
	// release fullname for the cluster agent's own ServiceAccount.
	ownServiceAccountSuffix = "-cluster-agent"
	// jobServiceAccountSuffix is the suffix the Helm chart appends to the
	// release fullname for the helm-actions Job's ServiceAccount.
	jobServiceAccountSuffix = "-helm-actions"
)

// jobServiceAccountName derives the name of the ServiceAccount the rollback
// Job must run as. The chart names it "<fullname>-helm-actions", and the
// cluster agent's own ServiceAccount "<fullname>-cluster-agent" — the release
// fullname isn't otherwise recoverable in Go (it depends on Helm's
// name/fullnameOverride logic), so it's derived by swapping the known suffix
// on the cluster agent's own ServiceAccount name instead.
func jobServiceAccountName(ctx context.Context, client kubernetes.Interface, ownNamespace string) (string, error) {
	podName, err := common.GetSelfPodName()
	if err != nil {
		return "", fmt.Errorf("get self pod name: %w", err)
	}

	pod, err := client.CoreV1().Pods(ownNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get self pod %s/%s: %w", ownNamespace, podName, err)
	}

	ownServiceAccount := pod.Spec.ServiceAccountName
	if ownServiceAccount == "" {
		return "", fmt.Errorf("pod %s/%s has no service account name", ownNamespace, podName)
	}

	fullname := strings.TrimSuffix(ownServiceAccount, ownServiceAccountSuffix)
	return fullname + jobServiceAccountSuffix, nil
}
