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

type DrainProcessor struct {
	drainProcessor        *drain.Drain
	drainLastTimeReported time.Time
	drainLastTimeUpdated  time.Time
	drainNLogs            int64
	id                    string
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
	}
}

func (d *DrainProcessor) Match(tokens []string) *drain.LogCluster {
	return d.drainProcessor.MatchFromTokens(tokens)
}

func (d *DrainProcessor) Train(tokens []string) {
	d.drainProcessor.TrainFromTokens(tokens)
}

func (d *DrainProcessor) MatchAndTrain(tokens []string) (*drain.LogCluster, bool) {
	start := time.Now()

	metrics.TlmDrainProcessed.Inc()
	d.drainNLogs++

	cluster := d.drainProcessor.MatchFromTokens(tokens)
	d.drainProcessor.TrainFromTokens(tokens)

	// Update if necessary
	if time.Since(d.drainLastTimeUpdated) < updateDrainInterval {
		d.update()
	}

	// Report if necessary
	if time.Since(d.drainLastTimeReported) >= reportDrainInfoInterval {
		d.ReportInfo()
	}

	toIgnore := cluster != nil && cluster.Size() >= drainClusterSizeThreshold
	if toIgnore {
		metrics.TlmDrainIgnored.Inc()
	}

	totalTimeDrainProcessing := time.Since(start).Nanoseconds()
	metrics.TlmDrainProcessTime.Set(float64(totalTimeDrainProcessing))

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
		log.Infof("drain(%s): Cluster #%d: %s", d.id, i+1, cluster.String())
	}
}

// Reports metrics and display logs for drain clusters
func (d *DrainProcessor) ReportInfo() {
	d.drainLastTimeReported = time.Now()

	clusters := d.drainProcessor.Clusters()
	drainClustersRatio := float64(len(clusters)) / float64(d.drainNLogs)
	log.Infof("drain(%s): %d clusters from %d logs (%f%%)", d.id, len(clusters), d.drainNLogs, drainClustersRatio*100)
	d.ShowClusters()

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
	metrics.TlmDrainMaxClusterRatio.Set(float64(maxSize) / float64(d.drainNLogs))
}

const (
	// Spaces are delimiters we want to remove
	drainTokenSpaces = " \t\n\r"
	// Delimiters are non spaces we want to use to split tokens but we want to keep them at the end of each token
	// TODO: Keep points? Commas?
	// drainTokenDelimiters = ":-._;/\\.,'\"`~*+=()[]{}&!@#$%^"
	drainTokenDelimiters = "[](){},;."
)

// TODO: Array of array of bytes?
func DrainTokenize(msg []byte) []string {
	tokens := make([]string, 0)
	token := make([]byte, 0)
	for _, char := range msg {
		// Skip spaces
		// TODO: Optimize by using []byte
		if strings.ContainsRune(drainTokenSpaces, rune(char)) {
			if len(token) > 0 {
				tokens = append(tokens, string(token))
				token = make([]byte, 0)
			}
			continue
		}

		token = append(token, char)

		// TODO: Merge delimiters?
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
