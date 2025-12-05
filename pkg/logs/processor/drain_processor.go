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

const reportDrainInfoInterval = 20 * time.Second

var (
	drainProcessor        *drain.Drain
	drainInitOnce         sync.Once
	drainMutex            sync.Mutex
	drainNLogs            int64
	drainLastTimeReported time.Time
)

func GetDrainProcessor() *drain.Drain {
	drainInitOnce.Do(func() {
		drainProcessor = drain.New(drain.DefaultConfig())
		drainLastTimeReported = time.Now()
		log.Info("Initialized drain processor")
	})
	return drainProcessor
}

func UseDrain() bool {
	return drainMutex.TryLock()
}

func ReleaseDrain() {
	if time.Since(drainLastTimeReported) >= reportDrainInfoInterval {
		reportDrainInfo()
		drainLastTimeReported = time.Now()
	}

	drainMutex.Unlock()
}

func CountLogs(n int64) {
	drainNLogs += n
}

// Reports metrics and display logs for drain clusters
func reportDrainInfo() {
	log.Infof("drain: %d clusters from %d logs (%f%%)", len(drainProcessor.Clusters()), drainNLogs, float64(len(drainProcessor.Clusters()))/float64(drainNLogs)*100)
	log.Infof("drain: Displaying the top 10 clusters")
	clusters := drainProcessor.Clusters()
	// Sort by size
	slices.SortFunc(clusters, func(a, b *drain.LogCluster) int {
		return a.Size() - b.Size()
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
	for _, cluster := range clusters {
		metrics.TlmDrainHistClusterSize.Observe(float64(cluster.Size()) / float64(maxSize))
	}
	metrics.TlmDrainClusters.Set(float64(len(clusters)))
	metrics.TlmDrainMaxClusterSize.Set(float64(maxSize))
	metrics.TlmDrainMaxClusterRatio.Set(float64(maxSize) / float64(drainNLogs))
}
