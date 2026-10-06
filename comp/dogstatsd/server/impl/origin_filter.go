// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/impl/parse"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// resolutionTTL is how long an origin to container ID resolution is reused.
	// It is kept short so that a restarted container (same pod UID and container
	// name, new container ID) is picked up quickly.
	resolutionTTL = 2 * time.Second
	// decisionTTL is how long a filtering decision for a container is reused.
	// Decisions are only cached once the container metadata is complete.
	decisionTTL = 30 * time.Second
	// maxOriginFilterCacheSize bounds each of the origin filter caches.
	maxOriginFilterCacheSize = 10000

	// metaCollectorCacheValidity is the cache validity passed to the meta collector,
	// matching the one used by the tagger.
	metaCollectorCacheValidity = time.Second
)

var containerIDFromSocketCutIndex = len(types.ContainerID) + types.GetSeparatorLength()

// originKey identifies an origin for the purpose of resolving its container.
type originKey struct {
	containerIDFromSocket string
	localContainerID      string
	inode                 uint64
	externalPodUID        string
	externalContainerName string
	externalInit          bool
}

type resolution struct {
	containerID string
	// resolved is handed to the tagger so it doesn't resolve the same data again.
	// It is shared by every sample with the same origin and must not be modified.
	resolved *taggertypes.ResolvedOrigin
	expires  time.Time
}

type decision struct {
	excluded bool
	expires  time.Time
}

// originFilter drops DogStatsD data based on the container it originates from,
// using the workloadfilter "dogstatsd" product rules.
//
// There is one originFilter per worker: it is not safe for concurrent use.
type originFilter struct {
	wmeta          workloadmeta.Component
	metaCollector  provider.MetaCollector
	bundle         workloadfilter.FilterBundle
	dropUnresolved bool

	resolutions map[originKey]resolution
	decisions   map[string]decision
	now         time.Time
}

// workloadFilterEnabled returns true if DogStatsD workload filtering should run:
// either some rules exist for the dogstatsd product or unresolved origins must be dropped.
func workloadFilterEnabled(cfg model.Reader) bool {
	if cfg.GetBool("dogstatsd_workload_filter_drop_unresolved") {
		return true
	}

	var bundles []workloadfilter.RuleBundle
	if err := structure.UnmarshalKey(cfg, "cel_workload_exclude", &bundles, structure.EnableStringUnmarshal); err != nil {
		return false
	}
	// Errors are reported by the workloadfilter component itself.
	productRules, _ := parse.GetProductConfigs(bundles)
	return len(productRules[workloadfilter.ProductDogstatsd][workloadfilter.ContainerType]) > 0
}

func newOriginFilter(wmeta workloadmeta.Component, filterStore workloadfilter.Component, metaCollector provider.MetaCollector, dropUnresolved bool) *originFilter {
	return &originFilter{
		wmeta:          wmeta,
		metaCollector:  metaCollector,
		bundle:         filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.ContainerCELDogstatsd}}),
		dropUnresolved: dropUnresolved,
		resolutions:    make(map[originKey]resolution),
		decisions:      make(map[string]decision),
		now:            time.Now(),
	}
}

// refreshClock updates the time used to expire cache entries. It is called once
// per batch of packets rather than once per sample.
func (f *originFilter) refreshClock() {
	f.now = time.Now()
}

// shouldDrop returns true if the data with the given origin must be dropped.
// It also attaches the container resolution to the origin so that the tagger
// reuses it instead of resolving it again.
func (f *originFilter) shouldDrop(origin *taggertypes.OriginInfo) bool {
	res := f.resolve(origin)
	if res.resolved != nil {
		origin.Resolved = res.resolved
	}

	if res.containerID == "" {
		return f.dropUnresolved
	}

	return f.isExcluded(res.containerID)
}

// resolve returns the origin container, using the same priority as the tagger:
// UDS origin, then client container ID (or inode), then external data.
// The inode and external data are only resolved when needed.
func (f *originFilter) resolve(origin *taggertypes.OriginInfo) resolution {
	key := originKey{
		containerIDFromSocket: origin.ContainerIDFromSocket,
		localContainerID:      origin.LocalData.ContainerID,
		inode:                 origin.LocalData.Inode,
		externalPodUID:        origin.ExternalData.PodUID,
		externalContainerName: origin.ExternalData.ContainerName,
		externalInit:          origin.ExternalData.Init,
	}

	if res, ok := f.resolutions[key]; ok && f.now.Before(res.expires) {
		return res
	}

	res := resolution{expires: f.now.Add(resolutionTTL)}
	switch {
	case len(origin.ContainerIDFromSocket) > containerIDFromSocketCutIndex:
		res.containerID = origin.ContainerIDFromSocket[containerIDFromSocketCutIndex:]
	case origin.LocalData.ContainerID != "":
		res.containerID = origin.LocalData.ContainerID
	default:
		resolved := &taggertypes.ResolvedOrigin{}
		if origin.LocalData.Inode != 0 && f.metaCollector != nil {
			resolved.InodeContainerID, _ = f.metaCollector.GetContainerIDForInode(origin.LocalData.Inode, metaCollectorCacheValidity)
			resolved.InodeDone = true
			res.containerID = resolved.InodeContainerID
		}
		if res.containerID == "" && origin.ExternalData.PodUID != "" && origin.ExternalData.ContainerName != "" && f.metaCollector != nil {
			resolved.ExternalDataContainerID, _ = f.metaCollector.ContainerIDForPodUIDAndContName(origin.ExternalData.PodUID, origin.ExternalData.ContainerName, origin.ExternalData.Init, metaCollectorCacheValidity)
			resolved.ExternalDataDone = true
			res.containerID = resolved.ExternalDataContainerID
		}
		if resolved.InodeDone || resolved.ExternalDataDone {
			res.resolved = resolved
		}
	}

	if pkglog.ShouldLog(pkglog.DebugLvl) {
		pkglog.Debugf("DogStatsD workload filter: origin %+v resolved to container %q", key, res.containerID)
	}

	if len(f.resolutions) >= maxOriginFilterCacheSize {
		clear(f.resolutions)
	}
	f.resolutions[key] = res
	return res
}

// isExcluded returns true if the given container matches the dogstatsd product rules.
//
// The decision is only cached once the container metadata is complete. Until then,
// the data is kept and every sample looks the container up again:
//   - the container is not known by workloadmeta yet,
//   - or it is a Kubernetes container that the kubelet has not linked to its pod yet,
//     so rules on pod properties can't match.
//
// Containers that don't run in a pod (e.g. Docker or ECS) have complete metadata
// without a pod, so their decision is cached.
func (f *originFilter) isExcluded(containerID string) bool {
	if d, ok := f.decisions[containerID]; ok && f.now.Before(d.expires) {
		return d.excluded
	}

	container, err := f.wmeta.GetContainer(containerID)
	if err != nil {
		if pkglog.ShouldLog(pkglog.TraceLvl) {
			pkglog.Tracef("DogStatsD workload filter: container %q not found in workloadmeta yet, keeping its data: %v", containerID, err)
		}
		return false
	}

	pod, podErr := f.wmeta.GetKubernetesPodForContainer(containerID)
	if podErr != nil && container.Labels[kubernetes.CriContainerNamespaceLabel] != "" {
		if pkglog.ShouldLog(pkglog.TraceLvl) {
			pkglog.Tracef("DogStatsD workload filter: container %q not linked to its pod yet, keeping its data: %v", containerID, podErr)
		}
		return false
	}

	d := decision{
		excluded: f.bundle.IsExcluded(workloadmetafilter.CreateContainer(container, workloadmetafilter.CreatePod(pod))),
		expires:  f.now.Add(decisionTTL),
	}
	pkglog.Debugf("DogStatsD workload filter: container %q (pod found: %t) excluded: %t", containerID, podErr == nil, d.excluded)

	if len(f.decisions) >= maxOriginFilterCacheSize {
		clear(f.decisions)
	}
	f.decisions[containerID] = d
	return d.excluded
}
