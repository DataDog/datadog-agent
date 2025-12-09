// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"slices"
	"sync"
	"time"

	"github.com/faceair/drain"

	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	reportDrainInfoInterval = 20 * time.Second
	updateDrainInterval     = 1 * time.Minute
	// We decrease each cluster size by this factor every time we update drain
	// 0.95 each minute ~= 50% in an hour
	drainClusterSizeDecay = 0.95
	// Threshold to determine whether or not to send a message based on the size of his cluster
	drainClusterSizeThreshold = 10
	drainMaxLineLength        = 160
)

var (
	drainProcessor        *drain.Drain
	drainInitOnce         sync.Once
	drainMutex            sync.Mutex
	drainNLogs            int64
	drainLastTimeReported time.Time
	drainLastTimeUpdated  time.Time
)

func GetDrainProcessor() *drain.Drain {
	drainInitOnce.Do(func() {
		drainProcessor = drain.New(drain.DefaultConfig())
		drainLastTimeReported = time.Now()
		drainLastTimeUpdated = time.Now()
		log.Info("Initialized drain processor")
	})
	return drainProcessor
}

func UseDrain() bool {
	canUse := drainMutex.TryLock()

	if canUse {
		updateDrain()
	}

	return canUse
}

func ReleaseDrain() {
	if time.Since(drainLastTimeReported) >= reportDrainInfoInterval {
		reportDrainInfo()
		drainLastTimeReported = time.Now()
	}

	drainMutex.Unlock()
}

// Decrease the size of each cluster by the decay factor
func updateDrain() {
	if time.Since(drainLastTimeUpdated) < updateDrainInterval {
		return
	}
	drainLastTimeUpdated = time.Now()

	clusters := drainProcessor.Clusters()
	for _, cluster := range clusters {
		// TODO: Can we remove clusters
		cluster.SetSize(int(float64(cluster.Size()) * drainClusterSizeDecay))
	}
}

// Reports metrics and display logs for drain clusters
func reportDrainInfo() {
	clusters := drainProcessor.Clusters()
	drainClustersRatio := float64(len(clusters)) / float64(drainNLogs)
	log.Infof("drain: %d clusters from %d logs (%f%%)", len(clusters), drainNLogs, drainClustersRatio*100)
	log.Infof("drain: Displaying the top 10 clusters")
	// Sort by size
	slices.SortFunc(clusters, func(a, b *drain.LogCluster) int {
		return b.Size() - a.Size()
	})
	nClusters := len(clusters)
	for i, cluster := range clusters[:min(10, nClusters)] {
		log.Infof("drain: Cluster #%d: %s", i+1, cluster.String())
	}

	maxSize := 0
	for _, cluster := range clusters {
		if cluster.Size() > maxSize {
			maxSize = cluster.Size()
		}
	}
	nClustersAboveThreshold := 0
	for _, cluster := range clusters {
		metrics.TlmDrainHistClusterSize.Observe(float64(cluster.Size()) / float64(maxSize) * 100)
		if cluster.Size() >= drainClusterSizeThreshold {
			nClustersAboveThreshold++
		}
	}
	metrics.TlmDrainClustersAboveThreshold.Set(float64(nClustersAboveThreshold))
	metrics.TlmDrainClusters.Set(float64(len(clusters)))
	metrics.TlmDrainClustersRatio.Set(drainClustersRatio)
	metrics.TlmDrainMaxClusterSize.Set(float64(maxSize))
	metrics.TlmDrainMaxClusterRatio.Set(float64(maxSize) / float64(drainNLogs))
}
