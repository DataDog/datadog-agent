// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	kubeletv1alpha1 "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

const (
	kubeletPodPath         = "/pods"
	kubeletMetricsPath     = "/metrics"
	kubeletStatsSummary    = "/stats/summary"
	authorizationHeaderKey = "Authorization"
	podListCacheKey        = "KubeletPodListCacheKey"
	unreadyAnnotation      = "ad.datadoghq.com/tolerate-unready"
	configSourceAnnotation = "kubernetes.io/config.source"
)

var (
	globalKubeUtil      *KubeUtil
	globalKubeUtilMutex sync.Mutex
)

// KubeUtil is a struct to hold the kubelet api url
// Instantiate with GetKubeUtil
type KubeUtil struct {
	// used to setup the KubeUtil
	initRetry retry.Retrier

	kubeletClient          *kubeletClient
	rawConnectionInfo      map[string]string // kept to pass to the python kubelet check
	podListCacheDuration   time.Duration
	filter                 *containers.Filter
	waitOnMissingContainer time.Duration
	podUnmarshaller        *podUnmarshaller
}

func (ku *KubeUtil) init() error {
	var err error
	ku.filter, err = containers.GetSharedMetricFilter()
	if err != nil {
		return err
	}

	ku.kubeletClient, err = getKubeletClient(context.Background())
	if err != nil {
		return err
	}

	ku.rawConnectionInfo["url"] = ku.kubeletClient.kubeletURL
	if ku.kubeletClient.config.scheme == "https" {
		ku.rawConnectionInfo["verify_tls"] = fmt.Sprintf("%v", ku.kubeletClient.config.tlsVerify)
		if ku.kubeletClient.config.caPath != "" {
			ku.rawConnectionInfo["ca_cert"] = ku.kubeletClient.config.caPath
		}
		if ku.kubeletClient.config.clientCertPath != "" && ku.kubeletClient.config.clientKeyPath != "" {
			ku.rawConnectionInfo["client_crt"] = ku.kubeletClient.config.clientCertPath
			ku.rawConnectionInfo["client_key"] = ku.kubeletClient.config.clientKeyPath
		}
		if ku.kubeletClient.config.token != "" {
			ku.rawConnectionInfo["token"] = ku.kubeletClient.config.token
		}
	}

	return nil
}

// NewKubeUtil returns a new KubeUtil
func NewKubeUtil() *KubeUtil {
	ku := &KubeUtil{
		rawConnectionInfo:    make(map[string]string),
		podListCacheDuration: config.Datadog.GetDuration("kubelet_cache_pods_duration") * time.Second,
		podUnmarshaller:      newPodUnmarshaller(),
	}

	waitOnMissingContainer := config.Datadog.GetDuration("kubelet_wait_on_missing_container")
	if waitOnMissingContainer > 0 {
		ku.waitOnMissingContainer = waitOnMissingContainer * time.Second
	}

	return ku
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

// GetKubeUtilWithRetrier returns an instance of KubeUtil or a retrier
func GetKubeUtilWithRetrier() (KubeUtilInterface, *retry.Retrier) {
	globalKubeUtilMutex.Lock()
	defer globalKubeUtilMutex.Unlock()
	if globalKubeUtil == nil {
		globalKubeUtil = NewKubeUtil()
		globalKubeUtil.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
			Name:              "kubeutil",
			AttemptMethod:     globalKubeUtil.init,
			Strategy:          retry.Backoff,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		})
	}
	err := globalKubeUtil.initRetry.TriggerRetry()
	if err != nil {
		log.Debugf("Kube util init error: %s", err)
		return nil, &globalKubeUtil.initRetry
	}
	return globalKubeUtil, nil
}

// GetKubeUtil returns an instance of KubeUtil.
func GetKubeUtil() (KubeUtilInterface, error) {
	util, retrier := GetKubeUtilWithRetrier()
	if retrier != nil {
		return nil, retrier.LastError()
	}
	return util, nil
}

// GetNodeInfo returns the IP address and the hostname of the first valid pod in the PodList
func (ku *KubeUtil) GetNodeInfo(ctx context.Context) (string, string, error) {
	pods, err := ku.GetLocalPodList(ctx)
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
func (ku *KubeUtil) GetNodename(ctx context.Context) (string, error) {
	pods, err := ku.GetLocalPodList(ctx)
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
func (ku *KubeUtil) GetLocalPodList(ctx context.Context) ([]*Pod, error) {
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

	data, code, err := ku.QueryKubelet(ctx, kubeletPodPath)
	if err != nil {
		return nil, errors.NewRetriable("podlist", fmt.Errorf("error performing kubelet query %s%s: %w", ku.kubeletClient.kubeletURL, kubeletPodPath, err))
	}
	if code != http.StatusOK {
		return nil, errors.NewRetriable("podlist", fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.kubeletClient.kubeletURL, kubeletPodPath, string(data)))
	}

	err = ku.podUnmarshaller.unmarshal(data, &pods)
	if err != nil {
		return nil, errors.NewRetriable("podlist", fmt.Errorf("unable to unmarshal podlist, invalid or null: %w", err))
	}

	// ensure we dont have nil pods
	tmpSlice := make([]*Pod, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if pod != nil {
			// Validate allocation size.
			// Limits hardcoded here are huge enough to never be hit.
			if len(pod.Status.Containers) > 10000 ||
				len(pod.Status.InitContainers) > 10000 {
				log.Errorf("Pod %s has a crazy number of containers: %d or init containers: %d. Skipping it!",
					pod.Metadata.UID, len(pod.Status.Containers), len(pod.Status.InitContainers))
				continue
			}
			allContainers := make([]ContainerStatus, 0, len(pod.Status.InitContainers)+len(pod.Status.Containers))
			allContainers = append(allContainers, pod.Status.InitContainers...)
			allContainers = append(allContainers, pod.Status.Containers...)
			pod.Status.AllContainers = allContainers
			tmpSlice = append(tmpSlice, pod)
		}
	}
	pods.Items = tmpSlice

	// cache the podList to reduce pressure on the kubelet
	cache.Cache.Set(podListCacheKey, pods, ku.podListCacheDuration)

	return pods.Items, nil
}

// ForceGetLocalPodList reset podList cache and call GetLocalPodList
func (ku *KubeUtil) ForceGetLocalPodList(ctx context.Context) ([]*Pod, error) {
	ResetCache()
	return ku.GetLocalPodList(ctx)
}

// GetPodForContainerID fetches the podList and returns the pod running
// a given container on the node. Reset the cache if needed.
// Returns a nil pointer if not found.
func (ku *KubeUtil) GetPodForContainerID(ctx context.Context, containerID string) (*Pod, error) {
	// Best case scenario
	pods, err := ku.GetLocalPodList(ctx)
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
		pods, err = ku.ForceGetLocalPodList(ctx)
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
			pods, err = ku.ForceGetLocalPodList(ctx)
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
	if containers.IsEntityName(containerID) {
		containerID = containers.ContainerIDForEntity(containerID)
	}

	for _, pod := range podList {
		for _, container := range pod.Status.GetAllContainers() {
			if container.ID != "" && containers.ContainerIDForEntity(container.ID) == containerID {
				return pod, nil
			}
		}
	}
	return nil, errors.NewNotFound(fmt.Sprintf("container %s in PodList", containerID))
}

// GetLocalStatsSummary returns node and pod stats from kubelet
func (ku *KubeUtil) GetLocalStatsSummary(ctx context.Context) (*kubeletv1alpha1.Summary, error) {
	data, code, err := ku.QueryKubelet(ctx, kubeletStatsSummary)
	if err != nil {
		return nil, errors.NewRetriable("statssummary", fmt.Errorf("error performing kubelet query %s%s: %w", ku.kubeletClient.kubeletURL, kubeletStatsSummary, err))
	}
	if code != http.StatusOK {
		return nil, errors.NewRetriable("statssummary", fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.kubeletClient.kubeletURL, kubeletStatsSummary, string(data)))
	}

	statsSummary := &kubeletv1alpha1.Summary{}
	if err := json.Unmarshal(data, statsSummary); err != nil {
		return nil, err
	}

	return statsSummary, nil
}

// QueryKubelet allows to query the KubeUtil registered kubelet API on the parameter path
// path commonly used are /healthz, /pods, /metrics
// return the content of the response, the response HTTP status code and an error in case of
func (ku *KubeUtil) QueryKubelet(ctx context.Context, path string) ([]byte, int, error) {
	return ku.kubeletClient.query(ctx, path)
}

// GetRawConnectionInfo returns a map containging the url and credentials to connect to the kubelet
// It refreshes the auth token on each call.
// Possible map entries:
//   - url: full url with scheme (required)
//   - verify_tls: "true" or "false" string
//   - ca_cert: path to the kubelet CA cert if set
//   - token: content of the bearer token if set
//   - client_crt: path to the client cert if set
//   - client_key: path to the client key if set
func (ku *KubeUtil) GetRawConnectionInfo() map[string]string {
	if ku.kubeletClient.config.scheme == "https" && ku.kubeletClient.config.token != "" {
		token, err := kubernetes.GetBearerToken(ku.kubeletClient.config.tokenPath)
		if err != nil {
			log.Warnf("Couldn't read auth token defined in %q: %v", ku.kubeletClient.config.tokenPath, err)
		} else {
			ku.rawConnectionInfo["token"] = token
		}
	}

	return ku.rawConnectionInfo
}

// GetRawMetrics returns the raw kubelet metrics payload
func (ku *KubeUtil) GetRawMetrics(ctx context.Context) ([]byte, error) {
	data, code, err := ku.QueryKubelet(ctx, kubeletMetricsPath)
	if err != nil {
		return nil, fmt.Errorf("error performing kubelet query %s%s: %s", ku.kubeletClient.kubeletURL, kubeletMetricsPath, err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.kubeletClient.kubeletURL, kubeletMetricsPath, string(data))
	}

	return data, nil
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
