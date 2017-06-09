package k8s

import (
	"context"
	"fmt"
	"os"

	"github.com/ericchiang/k8s"
	"github.com/ericchiang/k8s/api/v1"
)

// GetNodeInfo returns the IP address and the hostname of the node where
// this pod is running.
func GetNodeInfo() (ip, name string, err error) {
	pods, err := GetPodList()
	if err != nil {
		return "", "", fmt.Errorf("failed to retrieve host ip and name: %s", err)
	}

	for _, pod := range pods {
		// The env var HOSTNAME is set to the name of the pod running the datadog-agent
		if pod.GetMetadata().GetName() == os.Getenv("HOSTNAME") {
			// We identified the pod running the agent, let's return the host ip and hostname on which the pod is running
			return pod.GetStatus().GetHostIP(), pod.GetSpec().GetHostname(), nil
		}
	}

	return "", "", fmt.Errorf("Failed to fetch node info: could not identify the pod running the agent")
}

// GetPodList returns the list of pods running on the cluster where this pod is running
func GetPodList() ([]*v1.Pod, error) {
	client, err := k8s.NewInClusterClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %s", err)
	}

	pods, err := client.CoreV1().ListPods(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %s", err)
	}

	return pods.GetItems(), nil
}
