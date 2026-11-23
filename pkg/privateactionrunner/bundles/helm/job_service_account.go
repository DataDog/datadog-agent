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
	// carrying the release's resolved fullname.
	fullnameLabel = "app.kubernetes.io/name"
	// jobServiceAccountSuffix is the fixed suffix the chart appends to the
	// release fullname for the helm-actions Job's ServiceAccount
	jobServiceAccountSuffix = "-helm-actions"
)

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
