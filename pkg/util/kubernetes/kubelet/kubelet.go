// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package kubelet

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	log "github.com/cihub/seelog"
)

// Kubelet constants
const (
	KubeletHealthPath = "/healthz"
)

// KubeUtil is a struct to hold the kubelet api url
// Instanciate with NewKubeUtil
type KubeUtil struct {
	kubeletAPIURL string
}

// NewKubeUtil returns a new instance of KubeUtil.
func NewKubeUtil() (*KubeUtil, error) {
	kubeletURL, err := locateKubelet()
	if err != nil {
		return nil, fmt.Errorf("Could not find a way to connect to kubelet: %s", err)
	}

	kubeUtil := KubeUtil{
		kubeletAPIURL: kubeletURL,
	}
	return &kubeUtil, nil
}

// GetNodeInfo returns the IP address and the hostname of the node where
// this pod is running.
func (ku *KubeUtil) GetNodeInfo() (ip, name string, err error) {
	pods, err := ku.GetLocalPodList()
	if err != nil {
		return "", "", fmt.Errorf("Error getting pod list from kubelet: %s", err)
	}

	for _, pod := range pods {
		if !pod.Spec.HostNetwork {
			return pod.Status.HostIP, pod.Spec.NodeName, nil
		}
	}

	return "", "", fmt.Errorf("Failed to get node info")
}

// GetLocalPodList returns the list of pods running on the node where this pod is running
func (ku *KubeUtil) GetLocalPodList() ([]*Pod, error) {

	data, err := PerformKubeletQuery(fmt.Sprintf("%s/pods", ku.kubeletAPIURL))
	if err != nil {
		return nil, fmt.Errorf("Error performing kubelet query: %s", err)
	}

	v := new(PodList)
	if err := json.Unmarshal(data, v); err != nil {
		return nil, fmt.Errorf("Error unmarshalling json: %s", err)
	}

	return v.Items, nil
}

// GetPodForContainerID fetches the podlist and returns the pod running
// a given container on the node. Returns a nil pointer if not found.
func (ku *KubeUtil) GetPodForContainerID(containerID string) (*Pod, error) {
	pods, err := ku.GetLocalPodList()
	if err != nil {
		return nil, err
	}

	return ku.searchPodForContainerID(pods, containerID)
}

func (ku *KubeUtil) searchPodForContainerID(podlist []*Pod, containerID string) (*Pod, error) {
	if containerID == "" {
		return nil, errors.New("containerID is empty")
	}
	for _, pod := range podlist {
		for _, container := range pod.Status.Containers {
			if container.ID == containerID {
				return pod, nil
			}
		}
	}
	return nil, fmt.Errorf("container %s not found in podlist", containerID)
}

// Try and find the hostname to query the kubelet
// TODO: Add TLS verification
func locateKubelet() (string, error) {
	host := config.Datadog.GetString("kubernetes_kubelet_host")
	var err error

	if host == "" {
		host, err = docker.GetHostname()
		if err != nil {
			return "", fmt.Errorf("Unable to get hostname from docker, please set the kubernetes_kubelet_host option: %s", err)
		}
	}

	port := config.Datadog.GetInt("kubernetes_http_kubelet_port")
	url := fmt.Sprintf("http://%s:%d", host, port)
	healthzURL := fmt.Sprintf("%s%s", url, KubeletHealthPath)
	if _, err := PerformKubeletQuery(healthzURL); err == nil {
		return url, nil
	}
	log.Debugf("Couldn't query kubelet over HTTP, assuming it's not in no_auth mode.")

	port = config.Datadog.GetInt("kubernetes_https_kubelet_port")
	url = fmt.Sprintf("https://%s:%d", host, port)
	healthzURL = fmt.Sprintf("%s%s", url, KubeletHealthPath)
	if _, err := PerformKubeletQuery(healthzURL); err == nil {
		return url, nil
	}

	return "", fmt.Errorf("Could not find a method to connect to kubelet")
}

// PerformKubeletQuery performs a GET query against kubelet and return the response body
// Supports token-based auth
// TODO: TLS
func PerformKubeletQuery(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Could not create request: %s", err)
	}

	if strings.HasPrefix(url, "https") {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", kubernetes.GetAuthToken()))
	}

	res, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Error executing request to %s: %s", url, err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading response from %s: %s", url, err)
	}
	return body, nil
}
