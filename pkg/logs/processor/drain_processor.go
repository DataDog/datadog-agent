// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"slices"
	"strings"
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

type ClusterInfo struct {
	// Service of the logs that belong to this cluster
	Service string
	// First log occurrence of the cluster
	FirstOccurrence string
}

type DrainProcessor struct {
	drainProcessor        *drain.Drain
	drainLastTimeReported time.Time
	drainLastTimeUpdated  time.Time
	drainNLogs            int64
	id                    string
	// Used only for telemetry / tests, maps a cluster id to some infos
	clusterInfo map[int]ClusterInfo
}

func NewDrainProcessor(instanceID string, config *drain.Config) *DrainProcessor {
	if config == nil {
		config = drain.DefaultConfig()
	}

	return &DrainProcessor{
		drainProcessor:        drain.New(config),
		drainLastTimeReported: time.Now(),
		drainLastTimeUpdated:  time.Now(),
		drainNLogs:            0,
		id:                    instanceID,
		clusterInfo:           make(map[int]ClusterInfo),
	}
}

func (d *DrainProcessor) Match(tokens []string) *drain.LogCluster {
	return d.drainProcessor.MatchFromTokens(tokens)
}

func (d *DrainProcessor) Train(tokens []string) {
	d.drainProcessor.TrainFromTokens(tokens)
}

func (d *DrainProcessor) MatchAndTrain(log string, tokens []string, service string) (*drain.LogCluster, bool) {
	start := time.Now()

	metrics.TlmDrainProcessed.Inc("drain_id:" + d.id)
	d.drainNLogs++

	cluster := d.drainProcessor.MatchFromTokens(tokens)
	d.drainProcessor.TrainFromTokens(tokens)
	// TODO: Could be optimized
	if cluster == nil {
		cluster = d.drainProcessor.MatchFromTokens(tokens)
		// Only set first occurrence if it doesn't exist yet (preserve the true first occurrence)
		if _, exists := d.clusterInfo[cluster.ID()]; !exists {
			d.clusterInfo[cluster.ID()] = ClusterInfo{
				Service:         service,
				FirstOccurrence: log,
			}
		}
	}

	// Update if necessary
	if time.Since(d.drainLastTimeUpdated) >= updateDrainInterval {
		d.update()
	}

	// Report if necessary
	if time.Since(d.drainLastTimeReported) >= reportDrainInfoInterval {
		d.ReportInfo()
	}

	toIgnore := cluster != nil && cluster.Size() >= drainClusterSizeThreshold
	if toIgnore {
		metrics.TlmDrainIgnored.Inc("drain_id:" + d.id)
	}

	totalTimeDrainProcessing := time.Since(start).Nanoseconds()
	metrics.TlmDrainProcessTime.Set(float64(totalTimeDrainProcessing), "drain_id:"+d.id)

	return cluster, toIgnore
}

// Decrease the size of each cluster by the decay factor
func (d *DrainProcessor) update() {
	d.drainLastTimeUpdated = time.Now()

	clusters := d.drainProcessor.Clusters()
	for _, cluster := range clusters {
		// TODO: Can we remove clusters when size == 0?
		cluster.SetSize(max(1, int(float64(cluster.Size())*drainClusterSizeDecay)))
	}
}

func (d *DrainProcessor) ShowClusters() {
	clusters := d.drainProcessor.Clusters()
	log.Infof("drain(%s): Displaying the top 10 clusters", d.id)
	// Sort by size
	slices.SortFunc(clusters, func(a, b *drain.LogCluster) int {
		return b.Size() - a.Size()
	})
	nClusters := len(clusters)
	for i, cluster := range clusters[:min(10, nClusters)] {
		log.Infof("drain(%s): Cluster #%d (service: %s): %s", d.id, i+1, d.clusterInfo[cluster.ID()].Service, cluster.String())
	}
}

// Reports metrics and display logs for drain clusters
func (d *DrainProcessor) ReportInfo() {
	d.drainLastTimeReported = time.Now()

	mem := d.drainProcessor.MemoryUsage()
	clusters := d.drainProcessor.Clusters()
	drainClustersRatio := float64(len(clusters)) / float64(d.drainNLogs)
	log.Infof("drain(%s): %d clusters from %d logs (%f%%) - Memory: %dkB", d.id, len(clusters), d.drainNLogs, drainClustersRatio*100, (mem+1023)/1024)
	d.ShowClusters()

	maxSize := 0
	for _, cluster := range clusters {
		if cluster.Size() > maxSize {
			maxSize = cluster.Size()
		}
	}
	nClustersAboveThreshold := 0
	for _, cluster := range clusters {
		metrics.TlmDrainHistClusterSize.Observe(float64(cluster.Size())/float64(maxSize)*100, "drain_id:"+d.id)
		if cluster.Size() >= drainClusterSizeThreshold {
			nClustersAboveThreshold++
		}
	}
	clusterByService := make(map[string]int)
	for _, cluster := range clusters {
		clusterByService[d.clusterInfo[cluster.ID()].Service]++
	}
	for service, count := range clusterByService {
		metrics.TlmDrainClustersByService.Set(float64(count), "service:"+service, "drain_id:"+d.id)
	}
	metrics.TlmDrainClustersAboveThreshold.Set(float64(nClustersAboveThreshold), "drain_id:"+d.id)
	metrics.TlmDrainClusters.Set(float64(len(clusters)), "drain_id:"+d.id)
	metrics.TlmDrainClustersRatio.Set(drainClustersRatio, "drain_id:"+d.id)
	metrics.TlmDrainMaxClusterSize.Set(float64(maxSize), "drain_id:"+d.id)
	metrics.TlmDrainMaxClusterRatio.Set(float64(maxSize)/float64(d.drainNLogs), "drain_id:"+d.id)
	metrics.TlmDrainMemoryUsage.Set(float64(mem), "drain_id:"+d.id)
}

var (
	// Spaces are delimiters we want to remove
	drainTokenSpaces = " \t\n\r"
	// Delimiters are non spaces we want to use to split tokens but we want to keep them at the end of each token
	// TODO: Keep points? Commas?
	// drainTokenDelimiters = ":-._;/\\.,'\"`~*+=()[]{}&!@#$%^"
	drainTokenDelimiters      = "[](){},;."
	drainTokenDelimitersMerge = false
	// TODO
	drainTokenizeUsingSpace = true
)

// SetTokenizationConfig sets the tokenization configuration for the drain processor.
// This is primarily used by CLI commands to configure tokenization behavior.
func SetTokenizationConfig(useSpace bool, delimiters string, mergeDelimiters bool) {
	drainTokenizeUsingSpace = useSpace
	drainTokenDelimiters = delimiters
	drainTokenDelimitersMerge = mergeDelimiters
}

// TODO: Array of array of bytes?
func DrainTokenize(msg []byte) []string {
	if drainTokenizeUsingSpace {
		return strings.Split(string(msg), " ")
	}

	tokens := make([]string, 0)
	token := make([]byte, 0)
	for _, char := range msg {
		// Skip spaces
		// TODO: Optimize by using []byte
		if strings.ContainsRune(drainTokenSpaces, rune(char)) || (!drainTokenDelimitersMerge && strings.ContainsRune(drainTokenDelimiters, rune(char))) {
			if len(token) > 0 {
				tokens = append(tokens, string(token))
				token = make([]byte, 0)
			}
			continue
		}

		token = append(token, char)

		// Delimiters
		if strings.ContainsRune(drainTokenDelimiters, rune(char)) {
			// Keep delimiters at the end of the token
			if len(token) > 0 {
				tokens = append(tokens, string(token))
				token = make([]byte, 0)
			}
		}
	}

	if len(token) > 0 {
		tokens = append(tokens, string(token))
	}
	return tokens
}

func (d *DrainProcessor) Clusters() []*drain.LogCluster {
	return d.drainProcessor.Clusters()
}

// GetClusterInfo returns the ClusterInfo for a given cluster ID.
func (d *DrainProcessor) GetClusterInfo(clusterID int) ClusterInfo {
	return d.clusterInfo[clusterID]
}
