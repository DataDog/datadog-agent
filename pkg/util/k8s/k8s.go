package k8s

import (
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"os"
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
		if pod.GetName() == os.Getenv("HOSTNAME") {
			// We identified the pod running the agent, let's return the host ip and hostname on which the pod is running
			return pod.Status.HostIP, pod.Spec.Hostname, nil
		}
	}

	return "", "", fmt.Errorf("Failed to fetch node info: could not identify the pod running the agent")
}

// GetPodList returns the list of pods running on the cluster where this pod is running
func GetPodList() ([]v1.Pod, error) {

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve config: %s", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset: %s", err)
	}

	pods, err := clientset.Pods("").List(v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %s", err)
	}

	return pods.Items, nil
}
