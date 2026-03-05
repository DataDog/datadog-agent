// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogClusteringDetector_EmptyLog(t *testing.T) {
	d := &LogClusteringDetector{}
	result := d.Process(&logObs{content: []byte("")})
	assert.Empty(t, result.Metrics)
	assert.Empty(t, result.Anomalies)
}

func TestLogClusteringDetector_ClustersByTokenCount(t *testing.T) {
	d := &LogClusteringDetector{}

	// First log in a cluster triggers an anomaly.
	r1 := d.Process(&logObs{content: []byte("GET /foo 200"), tags: []string{"service:web"}, timestampMs: 1000})
	require.Len(t, r1.Metrics, 1)
	assert.Equal(t, "log.cluster.tokens_3.count", r1.Metrics[0].Name)
	assert.Equal(t, 1.0, r1.Metrics[0].Value)
	assert.Equal(t, []string{"service:web"}, r1.Metrics[0].Tags)
	require.Len(t, r1.Anomalies, 1)
	assert.Equal(t, "New log cluster: tokens_3", r1.Anomalies[0].Title)
	assert.Equal(t, int64(1), r1.Anomalies[0].Timestamp)

	// Second log in the same cluster does not trigger an anomaly.
	r2 := d.Process(&logObs{content: []byte("POST /bar 500"), tags: []string{"service:web"}})
	require.Len(t, r2.Metrics, 1)
	assert.Equal(t, "log.cluster.tokens_3.count", r2.Metrics[0].Name)
	assert.Empty(t, r2.Anomalies)

	// The cluster should have count=2 and keep the first exemplar.
	require.Contains(t, d.Clusters, 3)
	assert.Equal(t, int64(2), d.Clusters[3].Count)
	assert.Equal(t, "GET /foo 200", d.Clusters[3].Exemplar)
}

func TestLogClusteringDetector_DifferentClusters(t *testing.T) {
	d := &LogClusteringDetector{}

	d.Process(&logObs{content: []byte("hello world")})
	d.Process(&logObs{content: []byte("one two three four")})
	d.Process(&logObs{content: []byte("another pair")})

	assert.Len(t, d.Clusters, 2) // 2 tokens and 4 tokens
	assert.Equal(t, int64(2), d.Clusters[2].Count)
	assert.Equal(t, int64(1), d.Clusters[4].Count)
}
