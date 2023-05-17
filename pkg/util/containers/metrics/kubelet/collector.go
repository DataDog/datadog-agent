// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"hash/fnv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	kutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

const (
	contStatsCachePrefix    = "cs-"
	contNetStatsCachePrefix = "cns-"
	refreshCacheKey         = "refresh"

	kubeletCollectorID     = "kubelet"
	kubeletCallTimeout     = 10 * time.Second
	kubeletCacheGCInterval = 30 * time.Second
)

func init() {
	provider.GetProvider().RegisterCollector(provider.CollectorMetadata{
		ID: kubeletCollectorID,
		// Lowest priority as Kubelet stats are less detailed as we don't rely on cAdvisor
		Priority: 2,
		// Only runtimes implementing the CRI interface
		Runtimes: []string{provider.RuntimeNameCRIO, provider.RuntimeNameContainerd, provider.RuntimeNameDocker},
		Factory: func() (provider.Collector, error) {
			return newKubeletCollector()
		},
		DelegateCache: false,
	})
}

type kubeletCollector struct {
	kubeletClient kutil.KubeUtilInterface
	metadataStore workloadmeta.Store
	statsCache    provider.Cache
	refreshLock   sync.Mutex
}

func newKubeletCollector() (*kubeletCollector, error) {
	if !config.IsFeaturePresent(config.Kubernetes) {
		return nil, provider.ErrPermaFail
	}

	client, err := kutil.GetKubeUtil()
	if err != nil {
		return nil, provider.ConvertRetrierErr(err)
	}

	return &kubeletCollector{
		kubeletClient: client,
		statsCache:    *provider.NewCache(kubeletCacheGCInterval),
		metadataStore: workloadmeta.GetGlobalStore(),
	}, nil
}

// ID returns the collector ID.
func (kc *kubeletCollector) ID() string {
	return kubeletCollectorID
}

// GetContainerIDForPID returns a container ID for given PID.
// ("", nil) will be returned if no error but the containerd ID was not found.
func (kc *kubeletCollector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	// Not implemented
	return "", nil
}

// GetSelfContainerID returns the container ID for current container.
// ("", nil) will be returned if not possible to get ID for current container.
func (kc *kubeletCollector) GetSelfContainerID() (string, error) {
	return "", nil
}

// GetContainerStats returns stats by container ID.
func (kc *kubeletCollector) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	currentTime := time.Now()

	containerStats, found, err := kc.statsCache.Get(currentTime, contStatsCachePrefix+containerID, cacheValidity)
	if found {
		if containerStats != nil {
			return containerStats.(*provider.ContainerStats), err
		}
		return nil, err
	}

	// Item missing from cache
	if err := kc.refreshContainerCache(currentTime, cacheValidity); err != nil {
		return nil, err
	}

	containerStats, found, err = kc.statsCache.Get(currentTime, contStatsCachePrefix+containerID, cacheValidity)
	if found {
		if containerStats != nil {
			return containerStats.(*provider.ContainerStats), err
		}
		return nil, err
	}

	return nil, nil
}

// GetContainerPIDStats returns pid stats by container ID.
func (kc *kubeletCollector) GetContainerPIDStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerPIDStats, error) {
	// Not available
	return nil, nil
}

// GetContainerOpenFilesCount returns open files count by container ID.
func (kc *kubeletCollector) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) {
	// Not available
	return nil, nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (kc *kubeletCollector) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	currentTime := time.Now()

	containerNetworkStats, found, err := kc.statsCache.Get(currentTime, contNetStatsCachePrefix+containerID, cacheValidity)
	if found {
		if containerNetworkStats != nil {
			return containerNetworkStats.(*provider.ContainerNetworkStats), err
		}
		return nil, err
	}

	// Item missing from cache
	if err := kc.refreshContainerCache(currentTime, cacheValidity); err != nil {
		return nil, err
	}

	containerNetworkStats, found, err = kc.statsCache.Get(currentTime, contNetStatsCachePrefix+containerID, cacheValidity)
	if found {
		if containerNetworkStats != nil {
			return containerNetworkStats.(*provider.ContainerNetworkStats), err
		}
		return nil, err
	}

	return nil, nil
}

func (kc *kubeletCollector) refreshContainerCache(currentTime time.Time, cacheValidity time.Duration) error {
	kc.refreshLock.Lock()
	defer kc.refreshLock.Unlock()

	// Not refreshing if last refresh is within cacheValidity
	_, found, err := kc.statsCache.Get(currentTime, refreshCacheKey, cacheValidity)
	if found {
		return err
	}

	statsSummary, err := kc.getStatsSummary()
	if err == nil {
		kc.processStatsSummary(currentTime, statsSummary)
	} else {
		log.Debugf("Unable to get stats from Kubelet, err: %v", err)
	}

	kc.statsCache.Store(currentTime, refreshCacheKey, nil, err)
	return err
}

func (kc *kubeletCollector) getStatsSummary() (*v1alpha1.Summary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), kubeletCallTimeout)
	statsSummary, err := kc.kubeletClient.GetLocalStatsSummary(ctx)
	cancel()

	if err != nil {
		return nil, err
	}

	return statsSummary, err
}

func (kc *kubeletCollector) processStatsSummary(currentTime time.Time, statsSummary *v1alpha1.Summary) {
	if statsSummary == nil {
		return
	}

	for _, pod := range statsSummary.Pods {
		if len(pod.Containers) == 0 {
			continue
		}

		// Parsing network stats, need to store them by container anyway as it's the way it works currently
		// We use POD UID to generate an isolation group. It won't be network namespace FD, but it will still work
		// as all containers will get the same.
		// We could know if we're hostNetwork or not if we retrieve local POD list instead of relying on Workload meta,
		// albeit, with extra work. As this collector is designed to run in environment where we don't have access to
		// underlying host, it should not be an issue.
		podNetworkStats := &provider.ContainerNetworkStats{}
		convertNetworkStats(pod.Network, podNetworkStats)
		podNetworkStats.NetworkIsolationGroupID = pointer.Ptr(networkIDFromPODUID(pod.PodRef.UID))

		// As Metadata collector is running through polling, it can happen that we have newer PODs, containers
		metaPod, err := kc.metadataStore.GetKubernetesPod(pod.PodRef.UID)
		if err != nil || metaPod == nil {
			log.Debugf("Missing metadata for POD %s/%s - skipping, err: %v", pod.PodRef.Name, pod.PodRef.Namespace, err)
			continue
		}

		// In stats/summary we only have container name, need to remap to CID
		nameToCID := make(map[string]string, len(metaPod.Containers))
		for _, metaContainer := range metaPod.Containers {
			nameToCID[metaContainer.Name] = metaContainer.ID
		}

		// Parsing stats per container
		for _, container := range pod.Containers {
			cID := nameToCID[container.Name]
			if cID == "" {
				log.Debugf("Missing container ID for POD: %s/%s, container: %s - skipping, err: %v", pod.PodRef.Name, pod.PodRef.Namespace, container.Name, err)
				continue
			}

			containerStats := &provider.ContainerStats{}
			convertContainerStats(&container, containerStats)
			kc.statsCache.Store(currentTime, contStatsCachePrefix+cID, containerStats, nil)
			kc.statsCache.Store(currentTime, contNetStatsCachePrefix+cID, podNetworkStats, nil)
		}
	}
}

func convertContainerStats(kubeContainerStats *v1alpha1.ContainerStats, outContainerStats *provider.ContainerStats) {
	if kubeContainerStats == nil {
		return
	}

	if kubeContainerStats.CPU != nil {
		outContainerStats.Timestamp = kubeContainerStats.CPU.Time.Time
		outContainerStats.CPU = &provider.ContainerCPUStats{
			Total: pointer.UIntPtrToFloatPtr(kubeContainerStats.CPU.UsageCoreNanoSeconds),
		}
	}

	if kubeContainerStats.Memory != nil {
		outContainerStats.Memory = &provider.ContainerMemStats{
			UsageTotal: pointer.UIntPtrToFloatPtr(kubeContainerStats.Memory.UsageBytes),
			RSS:        pointer.UIntPtrToFloatPtr(kubeContainerStats.Memory.RSSBytes),
		}

		// On Linux `UsageBytes` is set. On Windows only `WorkingSetBytes` is set
		if outContainerStats.Memory.UsageTotal == nil && kubeContainerStats.Memory.WorkingSetBytes != nil {
			outContainerStats.Memory.UsageTotal = pointer.UIntPtrToFloatPtr(kubeContainerStats.Memory.WorkingSetBytes)
			outContainerStats.Memory.PrivateWorkingSet = pointer.UIntPtrToFloatPtr(kubeContainerStats.Memory.WorkingSetBytes)
		}
	}
}

func convertNetworkStats(podNetworkStats *v1alpha1.NetworkStats, outNetworkStats *provider.ContainerNetworkStats) {
	if podNetworkStats == nil {
		return
	}

	var sumBytesSent, sumBytesRcvd float64
	outNetworkStats.Timestamp = podNetworkStats.Time.Time
	outNetworkStats.Interfaces = make(map[string]provider.InterfaceNetStats, len(podNetworkStats.Interfaces))

	for _, interfaceStats := range podNetworkStats.Interfaces {
		fieldSet := false
		outInterfaceStats := provider.InterfaceNetStats{}

		if interfaceStats.TxBytes != nil {
			fieldSet = true
			sumBytesSent += float64(*interfaceStats.TxBytes)
			outInterfaceStats.BytesSent = pointer.UIntPtrToFloatPtr(interfaceStats.TxBytes)
		}
		if interfaceStats.RxBytes != nil {
			fieldSet = true
			sumBytesRcvd += float64(*interfaceStats.RxBytes)
			outInterfaceStats.BytesRcvd = pointer.UIntPtrToFloatPtr(interfaceStats.RxBytes)
		}

		if fieldSet {
			outNetworkStats.Interfaces[interfaceStats.Name] = outInterfaceStats
		}
	}

	if len(outNetworkStats.Interfaces) > 0 {
		outNetworkStats.BytesSent = &sumBytesSent
		outNetworkStats.BytesRcvd = &sumBytesRcvd
	}
}

func networkIDFromPODUID(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
