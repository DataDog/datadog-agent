// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkgErrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	devicepluginv1beta1 "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	podresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"
	kubeletv1alpha1 "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

const (
	kubeletConfigPath      = "/configz"
	kubeletPodPath         = "/pods"
	kubeletMetricsPath     = "/metrics"
	kubeletStatsSummary    = "/stats/summary"
	authorizationHeaderKey = "Authorization"
	podListCacheKey        = "KubeletPodListCacheKey"
	configSourceAnnotation = "kubernetes.io/config.source"
)

var (
	globalKubeUtil              *KubeUtil
	globalKubeUtilMutex         sync.Mutex
	errFailedKubeletClientHTTPS = errors.New("failed to use HTTPS for kubelet client")
)

// Time is used to mirror the wrapped Time struct inn"k8s.io/apimachinery/pkg/apis/meta/v1"
type Time struct {
	time.Time
}

// StreamLogOptions is used to mirror the options we need from PodLogOptions in "k8s.io/api/core/v1"
// without importing the entire package
type StreamLogOptions struct {
	SinceTime  *Time
	Follow     bool
	Timestamps bool
}

// KubeUtil is a struct to hold the kubelet api url
// Instantiate with GetKubeUtil
type KubeUtil struct {
	// used to setup the KubeUtil
	initRetry retry.Retrier

	// used to switch to HTTPS scheme if available
	httpsRetry retry.Retrier

	kubeletClient        [2]*kubeletClient
	kubeletClientIdx     atomic.Uint32
	rawConnectionInfo    map[string]string // kept to pass to the python kubelet check
	podListCacheDuration time.Duration     // a duration of 0 disables the cache
	podUnmarshaller      *podUnmarshaller
	podResourcesClient   *PodResourcesClient
	devicePluginsClient  DevicePluginClient

	useAPIServer bool

	// The node name is immutable in Kubernetes, so once it is fetched it should
	// be cached
	nodeName      string
	nodeNameMutex sync.Mutex
}

// kubelet client can be re-allocated at runtime this way we use a double bank system
// to allocate new client while other goroutines use the old one.
func (ku *KubeUtil) getKubeletClient() *kubeletClient {
	return ku.kubeletClient[ku.kubeletClientIdx.Load()%2]
}

func (ku *KubeUtil) initKubeletClientHTTPS() error {
	newKubeletClient, rawConnectionInfos, err := ku.initKubeletClient()
	if err != nil {
		return err
	}

	if newKubeletClient.config.scheme == "http" {
		return errFailedKubeletClientHTTPS
	}

	// we successfully allocated the new client for HTTPS
	// sotre it in the next slot
	newIndex := ku.kubeletClientIdx.Load() + 1
	ku.kubeletClient[newIndex] = newKubeletClient
	ku.rawConnectionInfo = rawConnectionInfos
	ku.kubeletClientIdx.Add(1)

	return nil
}

func (ku *KubeUtil) initKubeletClient() (*kubeletClient, map[string]string, error) {
	var err error
	var newKubeletClient *kubeletClient
	var rawConnectionInfo = map[string]string{}

	newKubeletClient, err = getKubeletClient(context.Background())
	if err != nil {
		return nil, nil, err
	}

	rawConnectionInfo["url"] = newKubeletClient.kubeletURL
	if newKubeletClient.config.scheme == "https" {
		rawConnectionInfo["verify_tls"] = strconv.FormatBool(newKubeletClient.config.tlsVerify)
		if newKubeletClient.config.caPath != "" {
			rawConnectionInfo["ca_cert"] = newKubeletClient.config.caPath
		}
		if newKubeletClient.config.clientCertPath != "" && newKubeletClient.config.clientKeyPath != "" {
			rawConnectionInfo["client_crt"] = newKubeletClient.config.clientCertPath
			rawConnectionInfo["client_key"] = newKubeletClient.config.clientKeyPath
		}
		if newKubeletClient.config.token != "" {
			rawConnectionInfo["token"] = newKubeletClient.config.token
		}
	}

	return newKubeletClient, rawConnectionInfo, nil
}

func (ku *KubeUtil) init() error {
	var err error

	ku.kubeletClient[0], ku.rawConnectionInfo, err = ku.initKubeletClient()
	if err != nil {
		return err
	}

	if pkgconfigsetup.Datadog().GetBool("kubelet_use_api_server") {
		ku.useAPIServer = true
		ku.getKubeletClient().config.nodeName, err = ku.GetNodename(context.Background())
		if err != nil {
			return err
		}
	}

	if env.IsFeaturePresent(env.PodResources) {
		ku.podResourcesClient, err = NewPodResourcesClient(pkgconfigsetup.Datadog())
		if err != nil {
			log.Warnf("Failed to create pod resources client, resource data will not be available: %s", err)
		}
	}

	if env.IsFeaturePresent(env.KubernetesDevicePlugins) {
		ku.devicePluginsClient, err = NewDevicePluginClient(pkgconfigsetup.Datadog())
		if err != nil {
			log.Warnf("Failed to create device plugins client, devices health will not be available: %s", err)
		}
	}

	return nil
}

// NewKubeUtil returns a new KubeUtil
func NewKubeUtil() *KubeUtil {
	return &KubeUtil{
		rawConnectionInfo:    make(map[string]string),
		podListCacheDuration: pkgconfigsetup.Datadog().GetDuration("kubelet_cache_pods_duration") * time.Second,
		podUnmarshaller:      newPodUnmarshaller(),
	}
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

		// prepare a retrier to switch from HTTP to HTTPS if available
		globalKubeUtil.httpsRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
			Name:              "kubeutil with HTTPS",
			AttemptMethod:     globalKubeUtil.initKubeletClientHTTPS, // call init(), returns an error until it uses HTTPS
			Strategy:          retry.Backoff,
			InitialRetryDelay: 10 * time.Second,
			MaxRetryDelay:     30 * time.Hour,
		})
	}
	err := globalKubeUtil.initRetry.TriggerRetry()
	if err != nil {
		log.Debugf("Kube util init error=%s", err.Error())
		return nil, &globalKubeUtil.initRetry
	}

	// try to switch to https
	if globalKubeUtil.getKubeletClient().config.scheme == "http" && time.Now().After(globalKubeUtil.httpsRetry.NextRetry()) {
		log.Infof("kubelet client uses http, try https instead")
		err := globalKubeUtil.httpsRetry.TriggerRetry()

		// no error => happy path, we have HTTPS now.
		// when the error is `errFailedKubeletClientHTTPS` this mean we succeed to have a kubeletClient
		// but it uses HTTP, we failed to reach kubelet using HTTPS.
		// we can still return the client as is, as it works using HTTP only.
		// the next call to this function will retry reaching it using HTTPS
		if err == nil || errors.Is(err, errFailedKubeletClientHTTPS) {
			log.Infof("failed to try https, http only for now")
			return globalKubeUtil, nil
		}

		log.Errorf("complete failure my friend: %s", err.Error())
		// error while init kubelet client
		return nil, &globalKubeUtil.httpsRetry
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

// StreamLogs connects to the kubelet and returns an open connection for the purposes of streaming container logs
func (ku *KubeUtil) StreamLogs(ctx context.Context, podNamespace, podName, containerName string, logOptions *StreamLogOptions) (io.ReadCloser, error) {
	query := fmt.Sprintf("follow=%t&timestamps=%t", logOptions.Follow, logOptions.Timestamps)
	if logOptions.SinceTime != nil {
		query += "&sinceTime=" + logOptions.SinceTime.Format(time.RFC3339)
	}
	path := fmt.Sprintf("/containerLogs/%s/%s/%s?%s", podNamespace, podName, containerName, query)
	return ku.getKubeletClient().queryWithResp(ctx, path)
}

// GetNodename returns the nodename
func (ku *KubeUtil) GetNodename(ctx context.Context) (string, error) {
	ku.nodeNameMutex.Lock()
	defer ku.nodeNameMutex.Unlock()

	if ku.nodeName != "" {
		return ku.nodeName, nil
	}

	var nodeName string

	if ku.useAPIServer {
		if ku.getKubeletClient().config.nodeName != "" {
			nodeName = ku.getKubeletClient().config.nodeName
		} else {
			stats, err := ku.GetLocalStatsSummary(ctx)
			if err == nil && stats.Node.NodeName != "" {
				nodeName = stats.Node.NodeName
			} else {
				return "", fmt.Errorf("failed to get kubernetes nodename from %s: %w", kubeletStatsSummary, err)
			}
		}
	} else {
		pods, err := ku.GetLocalPodList(ctx)
		if err != nil {
			return "", fmt.Errorf("error getting pod list from kubelet: %w", err)
		}

		for _, pod := range pods {
			if pod.Spec.NodeName != "" {
				nodeName = pod.Spec.NodeName
				break
			}
		}
		if nodeName == "" {
			return "", fmt.Errorf("failed to get the kubernetes nodename, pod list length: %d", len(pods))
		}
	}

	// Cache the node name, it's immutable
	ku.nodeName = nodeName

	return nodeName, nil
}

func (ku *KubeUtil) getLocalPodList(ctx context.Context) (*PodList, error) {
	var ok bool
	pods := PodList{}

	if ku.podListCacheDuration > 0 {
		if cached, hit := cache.Cache.Get(podListCacheKey); hit {
			pods, ok = cached.(PodList)
			if !ok {
				log.Errorf("Invalid pod list cache format, forcing a cache miss")
			} else {
				return &pods, nil
			}
		}
	}

	data, code, err := ku.QueryKubelet(ctx, kubeletPodPath)
	if err != nil {
		return nil, pkgErrors.NewRetriable("podlist", fmt.Errorf("error performing kubelet query %s%s: %w", ku.getKubeletClient().kubeletURL, kubeletPodPath, err))
	}
	if code != http.StatusOK {
		return nil, pkgErrors.NewRetriable("podlist", fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.getKubeletClient().kubeletURL, kubeletPodPath, string(data)))
	}

	err = ku.podUnmarshaller.unmarshal(data, &pods)
	if err != nil {
		return nil, pkgErrors.NewRetriable("podlist", fmt.Errorf("unable to unmarshal podlist, invalid or null: %w", err))
	}

	err = ku.addContainerResourcesData(ctx, pods.Items)
	if err != nil {
		log.Errorf("Error adding container resources data: %s", err)
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
			allContainers := make([]ContainerStatus, 0, len(pod.Status.InitContainers)+len(pod.Status.Containers)+len(pod.Status.EphemeralContainers))
			allContainers = append(allContainers, pod.Status.InitContainers...)
			allContainers = append(allContainers, pod.Status.Containers...)
			allContainers = append(allContainers, pod.Status.EphemeralContainers...)
			pod.Status.AllContainers = allContainers
			tmpSlice = append(tmpSlice, pod)
		}
	}
	pods.Items = tmpSlice

	if ku.podListCacheDuration > 0 {
		cache.Cache.Set(podListCacheKey, pods, ku.podListCacheDuration)
	}

	return &pods, nil
}

// addContainerResourcesData modifies the given pod list, populating the
// resources field of each container. If the pod resources API is not available,
// this is a no-op.
func (ku *KubeUtil) addContainerResourcesData(ctx context.Context, pods []*Pod) error {
	if ku.podResourcesClient == nil {
		return nil
	}

	containerToDevicesMap, err := ku.podResourcesClient.GetContainerToDevicesMap(ctx)
	if err != nil {
		return fmt.Errorf("error getting container resources data: %w", err)
	}

	for _, pod := range pods {
		ku.addResourcesToContainerList(containerToDevicesMap, pod, pod.Status.InitContainers)
		ku.addResourcesToContainerList(containerToDevicesMap, pod, pod.Status.Containers)
	}

	return nil
}

func (ku *KubeUtil) addResourcesToContainerList(containerToDevicesMap map[ContainerKey][]*podresourcesv1.ContainerDevices, pod *Pod, containers []ContainerStatus) {
	for i := range containers {
		container := &containers[i] // take the pointer so that we can modify the original
		key := ContainerKey{
			Namespace:     pod.Metadata.Namespace,
			PodName:       pod.Metadata.Name,
			ContainerName: container.Name,
		}
		devices, ok := containerToDevicesMap[key]
		if !ok {
			continue
		}

		for _, device := range devices {
			name := device.GetResourceName()
			for _, id := range device.GetDeviceIds() {
				container.ResolvedAllocatedResources = append(container.ResolvedAllocatedResources, ContainerAllocatedResource{
					Name: name,
					ID:   id,
				})
			}
		}
	}
}

// GetDevicesList returns the list of devices registered to the kubelet on the node.
// Information is cached for as configured by kubernetes_kubelet_deviceplugins_cache_duration
func (ku *KubeUtil) GetDevicesList(ctx context.Context) ([]*Device, error) {
	if ku.devicePluginsClient == nil {
		return nil, nil
	}

	if err := ku.devicePluginsClient.Refresh(ctx); err != nil {
		return nil, err
	}

	info, err := ku.devicePluginsClient.ListDevices(ctx)
	if err != nil {
		return nil, err
	}

	devices := []*Device{}
	for _, d := range info {
		devices = append(devices, &Device{
			ID:      d.ID,
			Healthy: d.Health == devicepluginv1beta1.Healthy,
		})
	}

	return devices, nil
}

// GetLocalPodList returns the list of pods running on the node.
// If kubernetes_pod_expiration_duration is set, old exited pods
// will be filtered out to keep the podlist size down: see json.go
func (ku *KubeUtil) GetLocalPodList(ctx context.Context) ([]*Pod, error) {
	pods, err := ku.getLocalPodList(ctx)
	if err != nil {
		return nil, err
	}

	return pods.Items, nil
}

// GetLocalPodListWithMetadata returns the list of pods running on the node,
// along with metadata surrounding that list, such as the number of pods
// which were expired and therefore removed from the list when it was generated.
func (ku *KubeUtil) GetLocalPodListWithMetadata(ctx context.Context) (*PodList, error) {
	return ku.getLocalPodList(ctx)
}

// GetLocalStatsSummary returns node and pod stats from kubelet
func (ku *KubeUtil) GetLocalStatsSummary(ctx context.Context) (*kubeletv1alpha1.Summary, error) {
	data, code, err := ku.QueryKubelet(ctx, kubeletStatsSummary)
	if err != nil {
		return nil, pkgErrors.NewRetriable("statssummary", fmt.Errorf("error performing kubelet query %s%s: %w", ku.getKubeletClient().kubeletURL, kubeletStatsSummary, err))
	}
	if code != http.StatusOK {
		return nil, pkgErrors.NewRetriable("statssummary", fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.getKubeletClient().kubeletURL, kubeletStatsSummary, string(data)))
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
	return ku.getKubeletClient().query(ctx, path)
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
	if ku.getKubeletClient().config.scheme == "https" && ku.getKubeletClient().config.token != "" {
		token, err := kubernetes.GetBearerToken(ku.getKubeletClient().config.tokenPath)
		if err != nil {
			log.Warnf("Couldn't read auth token defined in %q: %v", ku.getKubeletClient().config.tokenPath, err)
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
		return nil, fmt.Errorf("error performing kubelet query %s%s: %s", ku.getKubeletClient().kubeletURL, kubeletMetricsPath, err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.getKubeletClient().kubeletURL, kubeletMetricsPath, string(data))
	}

	return data, nil
}

// GetConfig returns the kubelet configuration from /configz
func (ku *KubeUtil) GetConfig(ctx context.Context) ([]byte, *ConfigDocument, error) {
	bytes, code, err := ku.QueryKubelet(ctx, kubeletConfigPath)
	if err != nil {
		return bytes, nil, fmt.Errorf("error performing kubelet query %s%s: %s", ku.getKubeletClient().kubeletURL, kubeletConfigPath, err)
	}
	if code != http.StatusOK {
		return bytes, nil, fmt.Errorf("unexpected status code %d on %s%s: %s", code, ku.getKubeletClient().kubeletURL, kubeletConfigPath, string(bytes))
	}

	var config *ConfigDocument
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		return bytes, nil, err
	}

	return bytes, config, nil
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
	if source, ok := pod.Metadata.Annotations[configSourceAnnotation]; ok && (source == "file" || source == "http") {
		return len(pod.Status.Containers) == 0
	}
	return false
}
