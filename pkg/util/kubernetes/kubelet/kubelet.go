// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"context"
	"crypto/tls"
	"expvar"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	kubeletPodPath         = "/pods"
	kubeletMetricsPath     = "/metrics"
	authorizationHeaderKey = "Authorization"
	podListCacheKey        = "KubeletPodListCacheKey"
	unreadyAnnotation      = "ad.datadoghq.com/tolerate-unready"
	configSourceAnnotation = "kubernetes.io/config.source"
)

var (
	globalKubeUtil      *KubeUtil
	globalKubeUtilMutex sync.Mutex
	kubeletExpVar       = expvar.NewInt("kubeletQueries")
)

// KubeUtil is a struct to hold the kubelet api url
// Instantiate with GetKubeUtil
type KubeUtil struct {
	sync.RWMutex
	// used to setup the KubeUtil
	initRetry retry.Retrier

	kubeletHost              string // resolved hostname or IPAddress
	kubeletAPIEndpoint       string // ${SCHEME}://${kubeletHost}:${PORT}
	kubeletProxyEnabled      bool
	kubeletAPIClient         *http.Client
	kubeletAPIRequestHeaders *http.Header
	rawConnectionInfo        map[string]string // kept to pass to the python kubelet check
	podListCacheDuration     time.Duration
	filter                   *containers.Filter
	waitOnMissingContainer   time.Duration
	podUnmarshaller          *podUnmarshaller
}

// ResetGlobalKubeUtil is a helper to remove the current KubeUtil global
// It is ONLY to be used for tests
func ResetGlobalKubeUtil() {
	globalKubeUtilMutex.Lock()
	defer globalKubeUtilMutex.Unlock()
	globalKubeUtil = nil
}

// ResetCache deletes existing kubeutil related cache
func ResetCache() {
	cache.Cache.Delete(podListCacheKey)
}

func newKubeUtil() *KubeUtil {
	ku := &KubeUtil{
		kubeletAPIClient:         &http.Client{Timeout: time.Second},
		kubeletAPIRequestHeaders: &http.Header{},
		rawConnectionInfo:        make(map[string]string),
		podListCacheDuration:     config.Datadog.GetDuration("kubelet_cache_pods_duration") * time.Second,
		podUnmarshaller:          newPodUnmarshaller(),
	}

	waitOnMissingContainer := config.Datadog.GetDuration("kubelet_wait_on_missing_container")
	if waitOnMissingContainer > 0 {
		ku.waitOnMissingContainer = waitOnMissingContainer * time.Second
	}

	return ku
}

// GetKubeUtil returns an instance of KubeUtil.
func GetKubeUtil() (KubeUtilInterface, error) {
	globalKubeUtilMutex.Lock()
	defer globalKubeUtilMutex.Unlock()
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

// GetNodename returns the nodename of the first pod.spec.nodeName in the PodList
func (ku *KubeUtil) GetNodename() (string, error) {
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

	return "", fmt.Errorf("failed to get the kubernetes nodename, pod list length: %d", len(pods))
}

// GetLocalPodList returns the list of pods running on the node.
// If kubernetes_pod_expiration_duration is set, old exited pods
// will be filtered out to keep the podlist size down: see json.go
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
		return nil, fmt.Errorf("error performing kubelet query %s%s: %s", ku.kubeletAPIEndpoint, kubeletPodPath, err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.kubeletAPIEndpoint, kubeletPodPath, string(data))
	}

	err = ku.podUnmarshaller.unmarshal(data, &pods)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal podlist, invalid or null: %s", err)
	}

	// ensure we dont have nil pods
	tmpSlice := make([]*Pod, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if pod != nil {
			tmpSlice = append(tmpSlice, pod)
		}
	}
	pods.Items = tmpSlice

	// cache the podList to reduce pressure on the kubelet
	cache.Cache.Set(podListCacheKey, pods, ku.podListCacheDuration)

	return pods.Items, nil
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
	// Best case scenario
	pods, err := ku.GetLocalPodList()
	if err != nil {
		return nil, err
	}
	pod, err := ku.searchPodForContainerID(pods, containerID)
	if err == nil {
		return pod, nil
	}

	// Retry with cache invalidation
	if err != nil && errors.IsNotFound(err) {
		log.Debugf("Cannot get container %q: %s, retrying without cache...", containerID, err)
		pods, err = ku.ForceGetLocalPodList()
		if err != nil {
			return nil, err
		}
		pod, err = ku.searchPodForContainerID(pods, containerID)
		if err == nil {
			return pod, nil
		}
	}

	// On some kubelet versions, containers can take up to a second to
	// register in the podlist, retry a few times before failing
	if ku.waitOnMissingContainer == 0 {
		log.Tracef("Still cannot get container %q, wait disabled", containerID)
		return pod, err
	}
	timeout := time.NewTimer(ku.waitOnMissingContainer)
	defer timeout.Stop()
	retryTicker := time.NewTicker(250 * time.Millisecond)
	defer retryTicker.Stop()
	for {
		log.Tracef("Still cannot get container %q: %s, retrying in 250ms", containerID, err)
		select {
		case <-retryTicker.C:
			pods, err = ku.ForceGetLocalPodList()
			if err != nil {
				continue
			}
			pod, err = ku.searchPodForContainerID(pods, containerID)
			if err != nil {
				continue
			}
			return pod, nil
		case <-timeout.C:
			// Return the latest error on timeout
			return nil, err
		}
	}
}

func (ku *KubeUtil) searchPodForContainerID(podList []*Pod, containerID string) (*Pod, error) {
	if containerID == "" {
		return nil, fmt.Errorf("containerID is empty")
	}

	// We will match only on the id itself, without runtime identifier, it should be quite unlikely on a Kube node
	// to have a container in the runtime used by Kube to match a container in another runtime...
	strippedContainerID := containers.ContainerIDForEntity(containerID)
	for _, pod := range podList {
		for _, container := range pod.Status.GetAllContainers() {
			if containers.ContainerIDForEntity(container.ID) == strippedContainerID {
				return pod, nil
			}
		}
	}
	return nil, errors.NewNotFound(fmt.Sprintf("container %s in PodList", containerID))
}

// GetStatusForContainerID returns the container status from the pod given an ID
func (ku *KubeUtil) GetStatusForContainerID(pod *Pod, containerID string) (ContainerStatus, error) {
	for _, container := range pod.Status.GetAllContainers() {
		if containerID == container.ID {
			return container, nil
		}
	}
	return ContainerStatus{}, fmt.Errorf("Container %v not found", containerID)
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

// GetPodForEntityID returns a pointer to the pod that corresponds to an entity ID.
// If the pod is not found it returns nil and an error.
func (ku *KubeUtil) GetPodForEntityID(entityID string) (*Pod, error) {
	if strings.HasPrefix(entityID, KubePodPrefix) {
		uid := strings.TrimPrefix(entityID, KubePodPrefix)
		return ku.GetPodFromUID(uid)
	}
	return ku.GetPodForContainerID(entityID)
}

// setupkubeletAPIClient will try to setup the http(s) client to query the kubelet
// with the following settings, in order:
//  - Load Certificate Authority if needed
//  - HTTPS w/ configured certificates
//  - HTTPS w/ configured token
//  - HTTPS w/ service account token
//  - HTTP (unauthenticated)
func (ku *KubeUtil) setupkubeletAPIClient() error {
	transport := &http.Transport{}
	err := ku.setupTLS(
		config.Datadog.GetBool("kubelet_tls_verify"),
		config.Datadog.GetString("kubelet_client_ca"),
		transport)
	if err != nil {
		log.Debugf("Failed to init tls, will try http only: %s", err)
		return nil
	}

	ku.kubeletAPIClient.Transport = transport
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
	ku.Lock()
	defer ku.Unlock()
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

	ku.Lock()
	defer ku.Unlock()
	ku.rawConnectionInfo["client_crt"] = crt
	ku.rawConnectionInfo["client_key"] = key

	return nil
}

func (ku *KubeUtil) setBearerToken(tokenPath string) error {
	token, err := kubernetes.GetBearerToken(tokenPath)
	if err != nil {
		return err
	}
	ku.Lock()
	defer ku.Unlock()
	ku.kubeletAPIRequestHeaders.Set("Authorization", fmt.Sprintf("bearer %s", token))
	ku.rawConnectionInfo["token"] = token
	return nil
}

func (ku *KubeUtil) resetCredentials() {
	ku.Lock()
	defer ku.Unlock()
	ku.kubeletAPIRequestHeaders.Del(authorizationHeaderKey)
	ku.rawConnectionInfo = make(map[string]string)
}

// QueryKubelet allows to query the KubeUtil registered kubelet API on the parameter path
// path commonly used are /healthz, /pods, /metrics
// return the content of the response, the response HTTP status code and an error in case of
func (ku *KubeUtil) QueryKubelet(path string) ([]byte, int, error) {
	var err error

	req := &http.Request{}
	req.Header = *ku.kubeletAPIRequestHeaders
	req.URL, err = url.Parse(fmt.Sprintf("%s%s", ku.kubeletAPIEndpoint, path))
	if err != nil {
		log.Debugf("Fail to create the kubelet request: %s", err)
		return nil, 0, err
	}

	response, err := ku.kubeletAPIClient.Do(req)
	kubeletExpVar.Add(1)
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

// GetKubeletAPIEndpoint returns the current endpoint used to perform QueryKubelet
func (ku *KubeUtil) GetKubeletAPIEndpoint() string {
	return ku.kubeletAPIEndpoint
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
	ku.Lock()
	defer ku.Unlock()
	if _, ok := ku.rawConnectionInfo["url"]; !ok {
		ku.rawConnectionInfo["url"] = ku.kubeletAPIEndpoint
	}
	return ku.rawConnectionInfo
}

// GetRawMetrics returns the raw kubelet metrics payload
func (ku *KubeUtil) GetRawMetrics() ([]byte, error) {
	data, code, err := ku.QueryKubelet(kubeletMetricsPath)
	if err != nil {
		return nil, fmt.Errorf("error performing kubelet query %s%s: %s", ku.kubeletAPIEndpoint, kubeletMetricsPath, err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.kubeletAPIEndpoint, kubeletMetricsPath, string(data))
	}

	return data, nil
}

// IsAgentHostNetwork returns whether the agent is running inside a container with `hostNetwork` or not
func (ku *KubeUtil) IsAgentHostNetwork() (bool, error) {
	cid, err := providers.ContainerImpl.GetAgentCID()
	if err != nil {
		return false, err
	}

	pod, err := ku.GetPodForContainerID(cid)
	if err != nil {
		return false, err
	}

	return pod.Spec.HostNetwork, nil
}

func (ku *KubeUtil) setupKubeletAPIEndpoint() error {
	// Proxied
	if ku.kubeletProxyEnabled {
		_, code, httpsURLErr := ku.QueryKubelet(kubeletPodPath)
		if httpsURLErr == nil {
			if code == http.StatusOK {
				log.Debugf("Kubelet endpoint is: %s", ku.kubeletAPIEndpoint)
				return nil
			}
			if code >= 500 {
				return fmt.Errorf("unexpected status code %d on endpoint %s%s", code, ku.kubeletAPIEndpoint, kubeletPodPath)
			}
			log.Warnf("Failed to securely reach the kubelet over HTTPS, received a status %d. Trying a non secure connection over HTTP. We highly recommend configuring TLS to access the kubelet", code)
		}
		log.Debugf("Cannot query %s%s: %s", ku.kubeletAPIEndpoint, kubeletPodPath, httpsURLErr)
	}
	// HTTPS
	ku.kubeletAPIEndpoint = fmt.Sprintf("https://%s:%d", ku.kubeletHost, config.Datadog.GetInt("kubernetes_https_kubelet_port"))
	_, code, httpsURLErr := ku.QueryKubelet(kubeletPodPath)
	if httpsURLErr == nil {
		if code == http.StatusOK {
			log.Debugf("Kubelet endpoint is: %s", ku.kubeletAPIEndpoint)
			return nil
		}
		if code >= 500 {
			return fmt.Errorf("unexpected status code %d on endpoint %s%s", code, ku.kubeletAPIEndpoint, kubeletPodPath)
		}
		log.Warnf("Failed to securely reach the kubelet over HTTPS, received a status %d. Trying a non secure connection over HTTP. We highly recommend configuring TLS to access the kubelet", code)
	}
	log.Debugf("Cannot query %s%s: %s", ku.kubeletAPIEndpoint, kubeletPodPath, httpsURLErr)

	// We don't want to carry the token in open http communication
	ku.resetCredentials()

	// HTTP
	ku.kubeletAPIEndpoint = fmt.Sprintf("http://%s:%d", ku.kubeletHost, config.Datadog.GetInt("kubernetes_http_kubelet_port"))
	_, code, httpURLErr := ku.QueryKubelet(kubeletPodPath)
	if httpURLErr == nil {
		if code == http.StatusOK {
			log.Debugf("Kubelet endpoint is: %s", ku.kubeletAPIEndpoint)
			return nil
		}
		return fmt.Errorf("unexpected status code %d on endpoint %s%s", code, ku.kubeletAPIEndpoint, kubeletPodPath)
	}
	log.Debugf("Cannot query %s%s: %s", ku.kubeletAPIEndpoint, kubeletPodPath, httpURLErr)

	return fmt.Errorf("cannot connect: https: %q, http: %q", httpsURLErr, httpURLErr)
}

// connectionInfo contains potential kubelet's ips and hostnames
type connectionInfo struct {
	ips       []string
	hostnames []string
}

func (ku *KubeUtil) init() error {
	var err error

	// Kubelet is unavailable, proxying calls through the APIServer - EKS on Fargate.
	ku.kubeletProxyEnabled = config.Datadog.GetBool("eks_fargate")

	// setting the kubeletHost
	kubeletHost := config.Datadog.GetString("kubernetes_kubelet_host")
	kubeletHTTPSPort := config.Datadog.GetInt("kubernetes_https_kubelet_port")
	kubeletHTTPPort := config.Datadog.GetInt("kubernetes_http_kubelet_port")
	potentialHosts := getPotentialKubeletHosts(kubeletHost)

	dedupeConnectionInfo(potentialHosts)

	err = ku.setKubeletHost(potentialHosts, kubeletHTTPSPort, kubeletHTTPPort)
	if err != nil {
		return err
	}

	err = ku.setupkubeletAPIClient()
	if err != nil {
		return err
	}

	ku.filter, err = containers.GetSharedFilter()
	if err != nil {
		return err
	}

	return ku.setupKubeletAPIEndpoint()
}

func getPotentialKubeletHosts(kubeletHost string) *connectionInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hosts := connectionInfo{ips: nil, hostnames: nil}
	if kubeletHost != "" {
		configIps, configHostnames := getKubeletHostFromConfig(ctx, kubeletHost)
		hosts.ips = append(hosts.ips, configIps...)
		hosts.hostnames = append(hosts.hostnames, configHostnames...)
		log.Debugf("Got potential kubelet connection info from config, ips: %v, hostnames: %v", configIps, configHostnames)
	}

	dockerIps, dockerHostnames := getKubeletHostFromDocker(ctx)
	hosts.ips = append(hosts.ips, dockerIps...)
	hosts.hostnames = append(hosts.hostnames, dockerHostnames...)
	log.Debugf("Got potential kubelet connection info from docker, ips: %v, hostnames: %v", dockerIps, dockerHostnames)

	return &hosts
}

func getKubeletHostFromConfig(ctx context.Context, kubeletHost string) ([]string, []string) {
	var ips []string
	var hostnames []string
	if kubeletHost == "" {
		log.Debug("kubernetes_kubelet_host is not set")
		return ips, hostnames
	}

	log.Debugf("Trying to parse kubernetes_kubelet_host: %s", kubeletHost)
	kubeletIP := net.ParseIP(kubeletHost)
	if kubeletIP == nil {
		log.Debugf("Parsing kubernetes_kubelet_host: %s is a hostname, cached, trying to resolve it to ip...", kubeletHost)
		hostnames = append(hostnames, kubeletHost)
		ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, kubeletHost)
		if err != nil {
			log.Debugf("Cannot LookupIP hostname %s: %v", kubeletHost, err)
		} else {
			log.Debugf("kubernetes_kubelet_host: %s is resolved to: %v", kubeletHost, ipAddrs)
			for _, ipAddr := range ipAddrs {
				ips = append(ips, ipAddr.IP.String())
			}
		}
	} else {
		log.Debugf("Parsed kubernetes_kubelet_host: %s is an address: %v, cached, trying to resolve it to hostname", kubeletHost, kubeletIP)
		ips = append(ips, kubeletIP.String())
		addrs, err := net.DefaultResolver.LookupAddr(ctx, kubeletHost)
		if err != nil {
			log.Debugf("Cannot LookupHost ip %s: %v", kubeletHost, err)
		} else {
			log.Debugf("kubernetes_kubelet_host: %s is resolved to: %v", kubeletHost, addrs)
			for _, addr := range addrs {
				hostnames = append(hostnames, addr)
			}
		}
	}

	return ips, hostnames
}

func getKubeletHostFromDocker(ctx context.Context) ([]string, []string) {
	var ips []string
	var hostnames []string
	dockerHost, err := docker.HostnameProvider()
	if err != nil {
		log.Debugf("unable to get hostname from docker, make sure to set the kubernetes_kubelet_host option: %s", err)
		return ips, hostnames
	}

	log.Debugf("Trying to resolve host name %s provided by docker to ip...", dockerHost)
	hostnames = append(hostnames, dockerHost)
	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, dockerHost)
	if err != nil {
		log.Debugf("Cannot resolve host name %s, cached, provided by docker to ip: %s", dockerHost, err)
	} else {
		log.Debugf("Resolved host name %s provided by docker to %v", dockerHost, ipAddrs)
		for _, ipAddr := range ipAddrs {
			ips = append(ips, ipAddr.IP.String())
		}
	}

	return ips, hostnames
}

func dedupeConnectionInfo(hosts *connectionInfo) {
	ipsKeys := make(map[string]bool)
	ips := []string{}
	for _, ip := range hosts.ips {
		if _, check := ipsKeys[ip]; !check {
			ipsKeys[ip] = true
			ips = append(ips, ip)
		}
	}

	hostnamesKeys := make(map[string]bool)
	hostnames := []string{}
	for _, hostname := range hosts.hostnames {
		if _, check := hostnamesKeys[hostname]; !check {
			hostnamesKeys[hostname] = true
			hostnames = append(hostnames, hostname)
		}
	}

	hosts.ips = ips
	hosts.hostnames = hostnames
}

// setKubeletHost select a kubelet host from potential kubelet hosts
// the method check HTTPS connections first and prioritize ips over hostnames
func (ku *KubeUtil) setKubeletHost(hosts *connectionInfo, httpsPort, httpPort int) error {
	var connectionErrors []error
	log.Debugf("Trying several connection methods to locate the kubelet...")
	if ku.kubeletProxyEnabled && config.Datadog.Get("kubernetes_kubelet_nodename") != "" {
		ku.kubeletAPIEndpoint = fmt.Sprintf("https://%s:%s/api/v1/nodes/%s/proxy/", os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT"), config.Datadog.Get("kubernetes_kubelet_nodename"))
		log.Infof("EKS on Fargate mode detected, will proxy calls to the Kubelet through the APIServer at %s", ku.kubeletAPIEndpoint)
		return nil
	}
	kubeletHost, errors := selectFromPotentialHostsHTTPS(ku, hosts.ips, httpsPort)
	if kubeletHost != "" && errors == nil {
		log.Infof("Connection to the kubelet succeeded! %s is set as kubelet host", kubeletHost)
		return nil
	}
	connectionErrors = append(connectionErrors, errors...)

	kubeletHost, errors = selectFromPotentialHostsHTTPS(ku, hosts.hostnames, httpsPort)
	if kubeletHost != "" && errors == nil {
		log.Infof("Connection to the kubelet succeeded! %s is set as kubelet host", kubeletHost)
		return nil
	}
	connectionErrors = append(connectionErrors, errors...)

	kubeletHost, errors = selectFromPotentialHostsHTTP(hosts.ips, httpPort)
	if kubeletHost != "" && errors == nil {
		ku.kubeletHost = kubeletHost
		log.Infof("Connection to the kubelet succeeded! %s is set as kubelet host", kubeletHost)
		return nil
	}
	connectionErrors = append(connectionErrors, errors...)

	kubeletHost, errors = selectFromPotentialHostsHTTP(hosts.hostnames, httpPort)
	if kubeletHost != "" && errors == nil {
		ku.kubeletHost = kubeletHost
		log.Infof("Connection to the kubelet succeeded! %s is set as kubelet host", kubeletHost)
		return nil
	}
	connectionErrors = append(connectionErrors, errors...)

	log.Debug("All connection attempts to the Kubelet failed.")
	return fmt.Errorf("cannot set a valid kubelet host: cannot connect to kubelet using any of the given hosts: %v %v, Errors: %v", hosts.ips, hosts.hostnames, connectionErrors)
}

func selectFromPotentialHostsHTTPS(ku *KubeUtil, hosts []string, httpsPort int) (string, []error) {
	var connectionErrors []error
	for _, host := range hosts {
		log.Debugf("Trying to use host %s with HTTPS", host)
		ku.kubeletHost = host
		err := ku.setupkubeletAPIClient()
		if err != nil {
			log.Debugf("Cannot setup https kubelet api client for %s: %v", host, err)
			connectionErrors = append(connectionErrors, err)
			continue
		}

		err = checkKubeletHTTPSConnection(ku, httpsPort)
		if err == nil {
			log.Debugf("Can connect to kubelet using %s and HTTPS", host)
			return host, nil
		}
		log.Debugf("Cannot connect to kubelet using %s and https: %v", host, err)
		connectionErrors = append(connectionErrors, err)
	}

	return "", connectionErrors
}

func selectFromPotentialHostsHTTP(hosts []string, httpPort int) (string, []error) {
	var connectionErrors []error
	for _, host := range hosts {
		log.Debugf("Trying to use host %s with HTTP", host)
		err := checkKubeletHTTPConnection(host, httpPort)
		if err == nil {
			log.Debugf("Can connect to kubelet using %s and HTTP", host)
			return host, nil
		}
		log.Debugf("Cannot connect to kubelet using %s and http: %v", host, err)
		connectionErrors = append(connectionErrors, err)
	}

	return "", connectionErrors
}

func checkKubeletHTTPSConnection(ku *KubeUtil, httpsPort int) error {
	c := http.Client{Timeout: time.Second}
	c.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	log.Debugf("Trying to query the kubelet endpoint %s ...", ku.kubeletAPIEndpoint)
	ku.kubeletAPIEndpoint = fmt.Sprintf("https://%s:%d", ku.kubeletHost, httpsPort)
	response, err := c.Get(ku.kubeletAPIEndpoint + "/")
	if err == nil {
		log.Infof("Successfully queried %s without any security settings, adding security transport settings to query %s%s", response.Request.URL, ku.kubeletAPIEndpoint, kubeletPodPath)

		response, err := ku.doKubeletRequest(kubeletPodPath)
		if err == nil {
			log.Infof("Successfully connected securely to kubelet endpoint %s", response.Request.URL)
			switch {
			case response.StatusCode == http.StatusOK:
				log.Infof("Successfully authorized to query the kubelet on %s: 200, using %s as kubelet endpoint", response.Request.URL, ku.kubeletAPIEndpoint)
				ku.resetCredentials()
				return nil

			case response.StatusCode >= http.StatusInternalServerError:
				log.Infof("Unexpected return code on request %s on kubelet endpoint %s", response.Request.URL, ku.kubeletAPIEndpoint)

			case response.StatusCode == http.StatusUnauthorized:
				log.Debugf("Unauthorized to request %s on kubelet endpoint %s, check the kubelet authentication/authorization settings", response.Request.URL, ku.kubeletAPIEndpoint)

			default:
				log.Debugf("Unexpected http code %d on kubelet endpoint %s", response.StatusCode, ku.kubeletAPIEndpoint)
			}

			// err != nil
		} else if strings.Contains(err.Error(), "x509: certificate is valid for") {
			log.Debugf(`Invalid x509 settings, the kubelet server certificate is not valid for this subject alternative name: %s, %v, Please check the SAN of the kubelet server certificate with "openssl x509 -in ${KUBELET_CERTIFICATE} -text -noout". `, ku.kubeletHost, err)
			return err

		} else if strings.Contains(err.Error(), "x509: certificate signed by unknown authority") {
			ku.RLock()
			certString, ok := ku.rawConnectionInfo["ca_cert"]
			ku.RUnlock()
			if !ok {
				certString = "<not-set>"
			}
			log.Debugf(`The kubelet server certificate is signed by unknown authority, the current cacert is %s. Is the kubelet issuing self-signed certificates? Please validate the kubelet certificate with "openssl verify -CAfile %s ${KUBELET_CERTIFICATE}" to avoid this error: %v`, certString, certString, err)
			return err

		} else {
			log.Debugf("Cannot query %s on kubelet endpoint %s: %v", kubeletPodPath, ku.kubeletAPIEndpoint, err)
			return err
		}
	} else {
		log.Debugf("Cannot use the HTTPS endpoint: %s", ku.kubeletAPIEndpoint)
	}

	ku.resetCredentials()
	return err
}

func (ku *KubeUtil) doKubeletRequest(path string) (*http.Response, error) {
	var err error
	req := &http.Request{}
	req.Header = *ku.kubeletAPIRequestHeaders
	req.URL, err = url.Parse(ku.kubeletAPIEndpoint + path)
	if err != nil {
		log.Debugf("Failed creating the kubelet request: %s", err)
		return nil, err
	}

	response, err := ku.kubeletAPIClient.Do(req)
	if err != nil {
		log.Debugf("Cannot request %s: %s", req.URL.String(), err)
		return nil, err
	}

	return response, nil
}

func checkKubeletHTTPConnection(kubeletHost string, httpPort int) error {
	// Trying connectivity insecurely with a dedicated client
	c := http.Client{Timeout: time.Second}

	// HTTP check
	if _, errHTTP := c.Get(fmt.Sprintf("http://%s:%d/", kubeletHost, httpPort)); errHTTP != nil {
		log.Debugf("Cannot connect through HTTP: %s", errHTTP)
		return fmt.Errorf("cannot connect: http: %q", errHTTP)
	}

	return nil
}

// IsPodReady return a bool if the Pod is ready
func IsPodReady(pod *Pod) bool {
	// static pods are always reported as Pending, so we make an exception there
	if pod.Status.Phase == "Pending" && isPodStatic(pod) {
		return true
	}

	if pod.Status.Phase != "Running" {
		return false
	}

	if tolerate, ok := pod.Metadata.Annotations[unreadyAnnotation]; ok && tolerate == "true" {
		return true
	}
	for _, status := range pod.Status.Conditions {
		if status.Type == "Ready" && status.Status == "True" {
			return true
		}
	}
	return false
}

// isPodStatic identifies whether a pod is static or not based on an annotation
// Static pods can be sent to the kubelet from files or an http endpoint.
func isPodStatic(pod *Pod) bool {
	if source, ok := pod.Metadata.Annotations[configSourceAnnotation]; ok == true && (source == "file" || source == "http") {
		return len(pod.Status.Containers) == 0
	}
	return false
}
