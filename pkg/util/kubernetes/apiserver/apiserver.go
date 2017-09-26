// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package apiserver

import (
	"context"
	"fmt"

	"github.com/ericchiang/k8s"
	"github.com/ericchiang/k8s/api/v1"
)

// GetGlobalPodList returns the list of pods running on the cluster where this pod is running
// This function queries the API server which could put heavy load on it so use with caution
func GetGlobalPodList() ([]*v1.Pod, error) {
	client, err := k8s.NewInClusterClient()
	if err != nil {
		return nil, fmt.Errorf("Failed to get client: %s", err)
	}

	pods, err := client.CoreV1().ListPods(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("Failed to get pods: %s", err)
	}

	return pods.GetItems(), nil
}
