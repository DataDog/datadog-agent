// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	kubeletPodPath         = "/pods"
	authorizationHeaderKey = "Authorization"
	podListCacheKey        = "KubeletPodListCacheKey"
)

var globalKubeUtil *KubeUtil

// KubeUtil is a struct to hold the kubelet api url
// Instantiate with GetKubeUtil
type KubeUtil struct {
	// used to setup the KubeUtil
	initRetry retry.Retrier

	kubeletHost              string // resolved hostname or IPAddress
	kubeletApiEndpoint       string // ${SCHEME}://${kubeletHost}:${PORT}
	kubeletApiClient         *http.Client
	kubeletApiRequestHeaders *http.Header
}

// ResetGlobalKubeUtil is a helper to remove the current KubeUtil global
// It is ONLY to be used for tests
func ResetGlobalKubeUtil() {
	globalKubeUtil = nil
}

// ResetCache deletes existing kubeutil related cache
func ResetCache() {
	cache.Cache.Delete(podListCacheKey)
}

func newKubeUtil() *KubeUtil {
	ku := &KubeUtil{
		kubeletApiClient:         &http.Client{Timeout: time.Second},
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

// HostnameProvider kubelet implementation for the hostname provider
func HostnameProvider(hostName string) (string, error) {
	ku, err := GetKubeUtil()
	if err != nil {
		return "", err
	}
	return ku.GetHostname()
}

// GetNodeInfo returns the IP address and the hostname of the first valid pod in the PodList
func (ku *KubeUtil) GetNodeInfo() (string, string, error) {
	pods, err := ku.GetLocalPodList()
	if err != nil {
		return "", "", fmt.Errorf("error getting pod list from kubelet: %s", err)
	}

	for _, pod := range pods {
		if !pod.Spec.HostNetwork {
			return pod.Status.HostIP, pod.Spec.NodeName, nil
		}
	}

	return "", "", fmt.Errorf("failed to get node info, pod list length: %d", len(pods))
}

// GetHostname returns the hostname of the first pod.spec.nodeName in the PodList
func (ku *KubeUtil) GetHostname() (string, error) {
	pods, err := ku.GetLocalPodList()
	if err != nil {
		return "", fmt.Errorf("error getting pod list from kubelet: %s", err)
	}

	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			continue
		}
		return pod.Spec.NodeName, nil
	}

	return "", fmt.Errorf("failed to get hostname, pod list length: %d", len(pods))
}

// GetLocalPodList returns the list of pods running on the node
func (ku *KubeUtil) GetLocalPodList() ([]*Pod, error) {
	var ok bool
	pods := PodList{}

	if cached, hit := cache.Cache.Get(podListCacheKey); hit {
		pods, ok = cached.(PodList)
		if !ok {
			log.Errorf("Invalid pod list cache format, forcing a cache miss")
		} else {
			return pods.Items, nil
		}
	}

	data, code, err := ku.QueryKubelet(kubeletPodPath)
	if err != nil {
		return nil, fmt.Errorf("error performing kubelet query %s%s: %s", ku.kubeletApiEndpoint, kubeletPodPath, err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.kubeletApiEndpoint, kubeletPodPath, string(data))
	}

	err = json.Unmarshal(data, &pods)
	if err != nil {
		return nil, err
	}

	// cache the podlist for 10 seconds to reduce pressure on the kubelet
	cacheDuration := 10 * time.Second
	if config.Datadog.GetBool("process_agent_enabled") {
		cacheDuration = 2 * time.Second
	}
	cache.Cache.Set(podListCacheKey, pods, cacheDuration)

	return pods.Items, nil
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

// setupKubeletApiClient will try to setup the http(s) client to query the kubelet
// with the following settings, in order:
//  - Load Certificate Authority if needed
//  - HTTPS w/ configured certificates
//  - HTTPS w/ configured token
//  - HTTPS w/ service account token
//  - HTTP (unauthenticated)
func (ku *KubeUtil) setupKubeletApiClient() error {
	tlsConfig, err := getTLSConfig()
	if err != nil {
		return err
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	ku.kubeletApiClient.Transport = transport

	switch {
	case isCertificatesConfigured():
		tlsConfig.Certificates, err = kubernetes.GetCertificates(
			config.Datadog.GetString("kubelet_client_crt"),
			config.Datadog.GetString("kubelet_client_key"),
		)
		return err

	case isTokenPathConfigured():
		return ku.setBearerToken(config.Datadog.GetString("kubelet_auth_token_path"))

	case kubernetes.IsServiceAccountTokenAvailable():
		return ku.setBearerToken(kubernetes.ServiceAccountTokenPath)

	default:
		// Without Token and without certificates
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

// QueryKubelet allows to query the KubeUtil registered kubelet API on the parameter path
// path commonly used are /healthz, /pods, /metrics
// return the content of the response, the response HTTP status code and an error in case of
func (ku *KubeUtil) QueryKubelet(path string) ([]byte, int, error) {
	var err error

	req := &http.Request{}
	req.Header = *ku.kubeletApiRequestHeaders
	req.URL, err = url.Parse(fmt.Sprintf("%s%s", ku.kubeletApiEndpoint, path))
	if err != nil {
		log.Debugf("Fail to create the kubelet request: %s", err)
		return nil, 0, err
	}

	response, err := ku.kubeletApiClient.Do(req)
	if err != nil {
		log.Debugf("Cannot request %s: %s", req.URL.String(), err)
		return nil, 0, err
	}
	defer response.Body.Close()

	b, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Debugf("Fail to read request %s body: %s", req.URL.String(), err)
		return nil, 0, err
	}
	log.Tracef("Successfully connected to %s, status code: %d, body len: %d", req.URL.String(), response.StatusCode, len(b))
	return b, response.StatusCode, nil
}

// GetKubeletApiEndpoint returns the current endpoint used to perform QueryKubelet
func (ku *KubeUtil) GetKubeletApiEndpoint() string {
	return ku.kubeletApiEndpoint
}

func (ku *KubeUtil) setupKubeletApiEndpoint() error {
	// HTTPS
	ku.kubeletApiEndpoint = fmt.Sprintf("https://%s:%d", ku.kubeletHost, config.Datadog.GetInt("kubernetes_https_kubelet_port"))
	_, code, httpsUrlErr := ku.QueryKubelet(kubeletPodPath)
	if httpsUrlErr == nil {
		if code == http.StatusOK {
			log.Debugf("Kubelet endpoint is: %s", ku.kubeletApiEndpoint)
			return nil
		}
		return fmt.Errorf("unexpected status code %d on endpoint %s%s", code, ku.kubeletApiEndpoint, kubeletPodPath)
	}
	log.Debugf("Cannot query %s%s: %s", ku.kubeletApiEndpoint, kubeletPodPath, httpsUrlErr)

	// We don't want to carry the token in open http communication
	ku.kubeletApiRequestHeaders.Del(authorizationHeaderKey)

	// HTTP
	ku.kubeletApiEndpoint = fmt.Sprintf("http://%s:%d", ku.kubeletHost, config.Datadog.GetInt("kubernetes_http_kubelet_port"))
	_, code, httpUrlErr := ku.QueryKubelet(kubeletPodPath)
	if httpUrlErr == nil {
		if code == http.StatusOK {
			log.Debugf("Kubelet endpoint is: %s", ku.kubeletApiEndpoint)
			return nil
		}
		return fmt.Errorf("unexpected status code %d on endpoint %s%s", code, ku.kubeletApiEndpoint, kubeletPodPath)
	}
	log.Debugf("Cannot query %s%s: %s", ku.kubeletApiEndpoint, kubeletPodPath, httpUrlErr)

	return fmt.Errorf("cannot connect: https: %q, http: %q", httpsUrlErr, httpUrlErr)
}

func (ku *KubeUtil) init() error {
	var err, errHTTPS, errHTTP error

	// setting the kubeletHost
	ku.kubeletHost = config.Datadog.GetString("kubernetes_kubelet_host")
	if ku.kubeletHost == "" {
		ku.kubeletHost, err = docker.HostnameProvider("")
		if err != nil {
			return fmt.Errorf("unable to get hostname from docker, please set the kubernetes_kubelet_host option: %s", err)
		}
	}

	// trying connectivity insecurely with a dedicated client
	c := http.Client{Timeout: time.Second}
	c.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}

	// HTTPS first
	_, errHTTPS = c.Get(fmt.Sprintf("https://%s:%d/", ku.kubeletHost, config.Datadog.GetInt("kubernetes_https_kubelet_port")))
	if errHTTPS != nil {
		log.Debugf("Cannot connect: %s, trying trough http", errHTTPS)
		// Only try the HTTP if HTTPS failed
		_, errHTTP = c.Get(fmt.Sprintf("http://%s:%d/", ku.kubeletHost, config.Datadog.GetInt("kubernetes_http_kubelet_port")))
	}

	if errHTTP != nil {
		log.Debugf("Cannot connect: %s", errHTTP)
		return fmt.Errorf("cannot connect: https: %q, http: %q", errHTTPS, errHTTP)
	}

	err = ku.setupKubeletApiClient()
	if err != nil {
		return err
	}
	return ku.setupKubeletApiEndpoint()
}
