// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	kubeletPodPath         = "/pods"
	kubeletMetricsPath     = "/metrics"
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
	rawConnectionInfo        map[string]string // kept to pass to the python kubelet check
	podListCacheDuration     time.Duration
	filter                   *containers.Filter
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
		rawConnectionInfo:        make(map[string]string),
		podListCacheDuration:     10 * time.Second,
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
		log.Debugf("Kube util init error: %s", err)
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
		if pod.Status.HostIP == "" || pod.Spec.NodeName == "" {
			continue
		}
		return pod.Status.HostIP, pod.Spec.NodeName, nil
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

	// cache the podList to reduce pressure on the kubelet
	cache.Cache.Set(podListCacheKey, pods, ku.podListCacheDuration)

	return pods.Items, nil
}

// SetPodListCacheDuration sets the podlist cache duration
func (ku *KubeUtil) SetPodListCacheDuration(duration time.Duration) {
	ku.podListCacheDuration = duration
}

// ForceGetLocalPodList reset podList cache and call GetLocalPodList
func (ku *KubeUtil) ForceGetLocalPodList() ([]*Pod, error) {
	ResetCache()
	return ku.GetLocalPodList()
}

// GetPodForContainerID fetches the podList and returns the pod running
// a given container on the node. Reset the cache if needed.
// Returns a nil pointer if not found.
func (ku *KubeUtil) GetPodForContainerID(containerID string) (*Pod, error) {
	pods, err := ku.GetLocalPodList()
	if err != nil {
		return nil, err
	}
	pod, err := ku.searchPodForContainerID(pods, containerID)
	if err != nil && errors.IsNotFound(err) {
		log.Debugf("Cannot get the containerID %q: %s, retrying without cache...", containerID, err)
		pods, err = ku.ForceGetLocalPodList()
		if err != nil {
			return nil, err
		}
		pod, err = ku.searchPodForContainerID(pods, containerID)
		if err != nil {
			return nil, err
		}
	}
	return pod, err
}

func (ku *KubeUtil) searchPodForContainerID(podList []*Pod, containerID string) (*Pod, error) {
	if containerID == "" {
		return nil, fmt.Errorf("containerID is empty")
	}
	for _, pod := range podList {
		for _, container := range pod.Status.Containers {
			if container.ID == containerID {
				return pod, nil
			}
		}
	}
	return nil, errors.NewNotFound(fmt.Sprintf("container %s in PodList", containerID))
}

func (ku *KubeUtil) GetPodFromUID(podUID string) (*Pod, error) {
	if podUID == "" {
		return nil, fmt.Errorf("pod UID is empty")
	}
	pods, err := ku.GetLocalPodList()
	if err != nil {
		return nil, err
	}
	for _, pod := range pods {
		if pod.Metadata.UID == podUID {
			return pod, nil
		}
	}
	log.Debugf("cannot get the pod uid %q: %s, retrying without cache...", podUID, err)

	pods, err = ku.ForceGetLocalPodList()
	if err != nil {
		return nil, err
	}
	for _, pod := range pods {
		if pod.Metadata.UID == podUID {
			return pod, nil
		}
	}
	return nil, fmt.Errorf("uid %s not found in pod list", podUID)
}

func (ku *KubeUtil) GetPodForEntityID(entityID string) (*Pod, error) {
	if strings.HasPrefix(entityID, KubePodPrefix) {
		uid := strings.TrimPrefix(entityID, KubePodPrefix)
		return ku.GetPodFromUID(uid)
	}
	return ku.GetPodForContainerID(entityID)
}

// setupKubeletApiClient will try to setup the http(s) client to query the kubelet
// with the following settings, in order:
//  - Load Certificate Authority if needed
//  - HTTPS w/ configured certificates
//  - HTTPS w/ configured token
//  - HTTPS w/ service account token
//  - HTTP (unauthenticated)
func (ku *KubeUtil) setupKubeletApiClient() error {
	transport := &http.Transport{}
	err := ku.setupTLS(
		config.Datadog.GetBool("kubelet_tls_verify"),
		config.Datadog.GetString("kubelet_client_ca"),
		transport)
	if err != nil {
		log.Debugf("Failed to init tls, will try http only: %s", err)
		return nil
	}

	ku.kubeletApiClient.Transport = transport
	switch {
	case isCertificatesConfigured():
		log.Debug("Using HTTPS with configured TLS certificates")
		return ku.setCertificates(
			config.Datadog.GetString("kubelet_client_crt"),
			config.Datadog.GetString("kubelet_client_key"),
			transport.TLSClientConfig)

	case isTokenPathConfigured():
		log.Debug("Using HTTPS with configured bearer token")
		return ku.setBearerToken(config.Datadog.GetString("kubelet_auth_token_path"))

	case kubernetes.IsServiceAccountTokenAvailable():
		log.Debug("Using HTTPS with service account bearer token")
		return ku.setBearerToken(kubernetes.ServiceAccountTokenPath)
	default:
		log.Debug("No configured token or TLS certificates, will try http only")
		return nil
	}
}

func (ku *KubeUtil) setupTLS(verifyTLS bool, caPath string, transport *http.Transport) error {
	if transport == nil {
		return fmt.Errorf("uninitialized http transport")
	}

	tlsConf, err := buildTLSConfig(verifyTLS, caPath)
	if err != nil {
		return err
	}
	transport.TLSClientConfig = tlsConf

	ku.rawConnectionInfo["verify_tls"] = fmt.Sprintf("%v", verifyTLS)
	if verifyTLS {
		ku.rawConnectionInfo["ca_cert"] = caPath
	}
	return nil
}

func (ku *KubeUtil) setCertificates(crt, key string, tlsConfig *tls.Config) error {
	if tlsConfig == nil {
		return fmt.Errorf("uninitialized TLS config")
	}
	certs, err := kubernetes.GetCertificates(crt, key)
	if err != nil {
		return err
	}
	tlsConfig.Certificates = certs

	ku.rawConnectionInfo["client_crt"] = crt
	ku.rawConnectionInfo["client_key"] = key

	return nil
}

func (ku *KubeUtil) setBearerToken(tokenPath string) error {
	token, err := kubernetes.GetBearerToken(tokenPath)
	if err != nil {
		return err
	}
	ku.kubeletApiRequestHeaders.Set("Authorization", fmt.Sprintf("bearer %s", token))
	ku.rawConnectionInfo["token"] = token
	return nil
}

func (ku *KubeUtil) resetCredentials() {
	ku.kubeletApiRequestHeaders.Del(authorizationHeaderKey)
	ku.rawConnectionInfo = make(map[string]string)
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

// GetConnectionInfo returns a map containging the url and credentials to connect to the kubelet
// Possible map entries:
//   - url: full url with scheme (required)
//   - verify_tls: "true" or "false" string
//   - ca_cert: path to the kubelet CA cert if set
//   - token: content of the bearer token if set
//   - client_crt: path to the client cert if set
//   - client_key: path to the client key if set
func (ku *KubeUtil) GetRawConnectionInfo() map[string]string {
	if _, ok := ku.rawConnectionInfo["url"]; !ok {
		ku.rawConnectionInfo["url"] = ku.kubeletApiEndpoint
	}
	return ku.rawConnectionInfo
}

// GetRawMetrics returns the raw kubelet metrics payload
func (ku *KubeUtil) GetRawMetrics() ([]byte, error) {
	data, code, err := ku.QueryKubelet(kubeletMetricsPath)
	if err != nil {
		return nil, fmt.Errorf("error performing kubelet query %s%s: %s", ku.kubeletApiEndpoint, kubeletMetricsPath, err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.kubeletApiEndpoint, kubeletMetricsPath, string(data))
	}

	return data, nil
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
		if code >= 500 {
			return fmt.Errorf("unexpected status code %d on endpoint %s%s", code, ku.kubeletApiEndpoint, kubeletPodPath)
		}
		log.Warnf("Failed to securely reach the kubelet over HTTPS, received a status %d. Trying a non secure connection over HTTP. We highly recommend configuring TLS to access the kubelet", code)
	}
	log.Debugf("Cannot query %s%s: %s", ku.kubeletApiEndpoint, kubeletPodPath, httpsUrlErr)

	// We don't want to carry the token in open http communication
	ku.resetCredentials()

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
	var err error

	// setting the kubeletHost
	ku.kubeletHost = config.Datadog.GetString("kubernetes_kubelet_host")
	if ku.kubeletHost == "" {
		ku.kubeletHost, err = docker.HostnameProvider("")
		if err != nil {
			return fmt.Errorf("unable to get hostname from docker, please set the kubernetes_kubelet_host option: %s", err)
		}
	}

	// Trying connectivity insecurely with a dedicated client
	c := http.Client{Timeout: time.Second}
	c.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}

	// HTTPS first
	if _, errHTTPS := c.Get(fmt.Sprintf("https://%s:%d/", ku.kubeletHost, config.Datadog.GetInt("kubernetes_https_kubelet_port"))); errHTTPS != nil {
		log.Debugf("Cannot connect through HTTPS: %s, trying through http", errHTTPS)

		// Only try the HTTP if HTTPS failed
		if _, errHTTP := c.Get(fmt.Sprintf("http://%s:%d/", ku.kubeletHost, config.Datadog.GetInt("kubernetes_http_kubelet_port"))); errHTTP != nil {
			log.Debugf("Cannot connect through HTTP: %s", errHTTP)
			return fmt.Errorf("cannot connect: https: %q, http: %q", errHTTPS, errHTTP)
		}
	}

	err = ku.setupKubeletApiClient()
	if err != nil {
		return err
	}

	ku.filter, err = containers.GetSharedFilter()
	if err != nil {
		return err
	}

	return ku.setupKubeletApiEndpoint()
}

// IsPodReady return a bool if the Pod is ready
func IsPodReady(pod *Pod) bool {
	if pod.Status.Phase != "Running" {
		return false
	}
	for _, status := range pod.Status.Conditions {
		if status.Type == "Ready" && status.Status == "True" {
			return true
		}
	}
	return false
}
