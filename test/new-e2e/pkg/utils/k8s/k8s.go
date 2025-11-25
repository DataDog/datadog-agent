// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package k8s contains utility functions for interacting with Kubernetes clusters.
package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
)

// CheckJobErrors checks if a job has failed and returns an error with more details about the failure, such as the pod that failed and the error code.
func CheckJobErrors(ctx context.Context, client kubernetes.Interface, namespace, jobName string) error {
	job, err := client.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting job %s: %w", jobName, err)
	}

	if job.Status.Failed == 0 {
		return nil
	}

	// Get pod logs for debugging
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("job-name", jobName).String(),
		Limit:         1,
	})
	if err != nil {
		return fmt.Errorf("error listing pods for job %s: %w", jobName, err)
	}

	// Try to find a pod that failed to return a specific error about what happened
	for _, pod := range pods.Items {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode != 0 {
				return fmt.Errorf("workload job %s failed: pod %s container %s exited with code %d: %s",
					jobName, pod.Name, containerStatus.Name,
					containerStatus.State.Terminated.ExitCode,
					containerStatus.State.Terminated.Message)
			}
		}
	}

	// Could not find a specific pod that failed, so return a generic error
	return fmt.Errorf("workload job %s failed. Kubernetes reports %d failed pods, but we could not find a specific one with the failure", jobName, job.Status.Failed)
}
