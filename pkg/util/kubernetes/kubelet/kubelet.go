// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var globalKubeUtil *KubeUtil

// KubeUtil is a struct to hold the kubelet api url
// Instantiate with GetKubeUtil
type KubeUtil struct {
	// used to setup the KubeUtil
	initRetry retry.Retrier

	kubeletApiEndpoint       string
	kubeletApiClient         *http.Client
	kubeletApiRequestHeaders *http.Header
}

// ResetGlobalKubeUtil is a helper to remove the current KubeUtil global
// It should be called essentially in the tests
func ResetGlobalKubeUtil() {
	globalKubeUtil = nil
}

func newKubeUtil() *KubeUtil {
	ku := &KubeUtil{
		kubeletApiClient:         &http.Client{},
		kubeletApiRequestHeaders: &http.Header{},
	}
	return ku
}

// GetKubeUtil returns an instance of KubeUtil.
func GetKubeUtil() (*KubeUtil, error) {
	if globalKubeUtil == nil {
		globalKubeUtil = newKubeUtil()
		globalKubeUtil.initRetry.SetupRetrier(&retry.Config{
			Name:          "kubeutil",
			AttemptMethod: globalKubeUtil.init,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	}
	err := globalKubeUtil.initRetry.TriggerRetry()
	if err != nil {
		log.Debugf("Init error: %s", err)
		return nil, err
	}
	return globalKubeUtil, nil
}

// GetNodeInfo returns the IP address and the hostname of the node where
// this pod is running.
func (ku *KubeUtil) GetNodeInfo() (ip, name string, err error) {
	pods, err := ku.GetLocalPodList()
	if err != nil {
		return "", "", fmt.Errorf("error getting pod list from kubelet: %s", err)
	}

	for _, pod := range pods {
		if !pod.Spec.HostNetwork {
			return pod.Status.HostIP, pod.Spec.NodeName, nil
		}
	}

	return "", "", fmt.Errorf("failed to get node info")
}

// GetLocalPodList returns the list of pods running on the node where this pod is running
func (ku *KubeUtil) GetLocalPodList() ([]*Pod, error) {
	data, err := ku.QueryKubelet("/pods")
	if err != nil {
		return nil, fmt.Errorf("error performing kubelet query: %s", err)
	}

	v := &PodList{}
	err = json.Unmarshal(data, v)
	if err != nil {
		return nil, err
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

// setupKubeletApiClient will try to setup the http client to query the kubelet
// with the following settings, in order:
//  - HTTPS w/ configured certificates
//  - HTTPS w/ configured token
//  - HTTPS w/ service account token
//  - HTTP
func (ku *KubeUtil) setupKubeletApiClient() error {
	tlsConfig, err := getTLSConfig()
	if err != nil {
		return err
	}
	transport := http.Transport{
		TLSClientConfig: tlsConfig,
	}
	ku.kubeletApiClient.Transport = &transport

	switch {
	case isConfiguredCertificates():
		tlsConfig.Certificates, err = kubernetes.GetCertificates(
			config.Datadog.GetString("kubelet_client_crt"),
			config.Datadog.GetString("kubelet_client_key"),
		)
		return err

	case isConfiguredTokenPath():
		return ku.setBearerToken(config.Datadog.GetString("kubelet_auth_token_path"))

	case kubernetes.IsServiceAccountToken():
		return ku.setBearerToken(kubernetes.ServiceAccountTokenPath)

		// Without Token and without certificates
	default:
		return nil
	}
}

func (ku *KubeUtil) setBearerToken(tokenPath string) error {
	token, err := kubernetes.GetBearerToken(tokenPath)
	if err != nil {
		return err
	}
	ku.kubeletApiRequestHeaders.Set("Authorization", token)
	return nil
}

func (ku *KubeUtil) createKubeletRequest(path string) (*http.Request, error) {
	var err error

	req := &http.Request{}
	req.Header = *ku.kubeletApiRequestHeaders
	req.URL, err = url.Parse(fmt.Sprintf("%s%s", ku.kubeletApiEndpoint, path))
	return req, err
}

func (ku *KubeUtil) QueryKubelet(path string) ([]byte, error) {
	req, err := ku.createKubeletRequest(path)
	if err != nil {
		log.Debugf("Fail to create the kubelet request: %s", err)
		return nil, err
	}
	response, err := ku.kubeletApiClient.Do(req)
	if err != nil {
		log.Debugf("Fail to request %s: %s", req.URL.String(), err)
		return nil, err
	}
	defer response.Body.Close()
	log.Debugf("Successfully connected to %s, reading body", req.URL.String())
	return ioutil.ReadAll(response.Body)
}

// GetKubeletApiEndpoint returns the current endpoint used to perform QueryKubelet
func (ku *KubeUtil) GetKubeletApiEndpoint() string {
	return ku.kubeletApiEndpoint
}

func (ku *KubeUtil) setupKubeletApiEndpoint() error {
	var err error

	kubeHost := config.Datadog.GetString("kubernetes_kubelet_host")
	if kubeHost == "" {
		kubeHost, err = docker.HostnameProvider("")
		if err != nil {
			return fmt.Errorf("unable to get hostname from docker, please set the kubernetes_kubelet_host option: %s", err)
		}
	}

	// HTTPS
	ku.kubeletApiEndpoint = fmt.Sprintf("https://%s:%d", kubeHost, config.Datadog.GetInt("kubernetes_https_kubelet_port"))
	_, httpsUrlErr := ku.QueryKubelet("/pods")
	if httpsUrlErr == nil {
		return nil
	}
	log.Debugf("Fail to use %s: %s", ku.kubeletApiEndpoint, httpsUrlErr)

	// We don't want to carry the token in open http communication
	ku.kubeletApiRequestHeaders.Del("Authorization")

	// HTTP
	ku.kubeletApiEndpoint = fmt.Sprintf("http://%s:%d", kubeHost, config.Datadog.GetInt("kubernetes_http_kubelet_port"))
	_, httpUrlErr := ku.QueryKubelet("/pods")
	if httpUrlErr == nil {
		return nil
	}
	log.Debugf("Fail to use %s: %s", ku.kubeletApiEndpoint, httpUrlErr)

	return fmt.Errorf("no valid API endpoint for the kubelet: https: %q, http: %q", httpsUrlErr, httpUrlErr)
}

func (ku *KubeUtil) init() error {
	err := ku.setupKubeletApiClient()
	if err != nil {
		return err
	}
	return ku.setupKubeletApiEndpoint()
}
