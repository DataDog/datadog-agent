// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// KubernetesClient is a wrapper around the k8s client library and provides convenience methods for interacting with a
// k8s cluster
type KubernetesClient struct {
	K8sConfig *rest.Config
	K8sClient kubeClient.Interface
}

// NewKubernetesClient creates a new KubernetesClient
func NewKubernetesClient(config *rest.Config) (*KubernetesClient, error) {
	// Create client
	k8sClient, err := kubeClient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &KubernetesClient{
		K8sConfig: config,
		K8sClient: k8sClient,
	}, nil
}

// PodExec execs into a given namespace/pod and returns the output for the given command
func (k *KubernetesClient) PodExec(namespace, pod, container string, cmd []string) (stdout, stderr string, err error) {
	req := k.K8sClient.CoreV1().RESTClient().Post().Resource("pods").Namespace(namespace).Name(pod).SubResource("exec")
	option := &corev1.PodExecOptions{
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
		Container: container,
		Command:   cmd,
	}

	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(k.K8sConfig, "POST", req.URL())
	if err != nil {
		return "", "", err
	}

	var stdoutSb, stderrSb strings.Builder
	err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdoutSb,
		Stderr: &stderrSb,
	})
	if err != nil {
		return "", "", err
	}

	return stdoutSb.String(), stderrSb.String(), nil
}
