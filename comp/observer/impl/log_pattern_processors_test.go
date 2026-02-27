// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"hash"
	"hash/fnv"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
func TestWatchdogLogAnomalyDetector(t *testing.T) {
	observationsChannel := make(chan observation, 1024)
	// Each group is a batch, there is a time step between each group
	logs := [][]*message.Message{
		{
			message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 200 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("this is my test 2"), nil, "info", time.Now().Unix()),
		},
		{
			message.NewMessage([]byte("this is my test 0"), nil, "info", time.Now().Unix()),
		},
		{
			message.NewMessage([]byte("this is my test 0"), nil, "info", time.Now().Unix()),
		},
		{
			message.NewMessage([]byte("this is my test 0"), nil, "info", time.Now().Unix()),
		},
		{
			message.NewMessage([]byte("this is my test 0"), nil, "info", time.Now().Unix()),
		},
		{
			message.NewMessage([]byte("this is my test 0"), nil, "info", time.Now().Unix()),
		},
		{
			message.NewMessage([]byte("http 200 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
		},
		{
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
		},
		{
			message.NewMessage([]byte("http 200 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
		},
		{
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 200 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
			message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
		},
		{
			message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
		},
	}

	// Add tags
	for _, log := range logs {
		for _, log := range log {
			log.ProcessingTags = []string{"dirname:/tmp", "filename:a.log"}
		}
	}
	logs[len(logs)-1][0].ProcessingTags = []string{"dirname:/var", "filename:b.log"}

	const nCpus = 4
	pipelineChannel := make(chan *patterns.MultiThreadResult[*LogADTags], 1024)
	pipeline := patterns.NewMultiThreadPipeline[*LogADTags](nCpus, pipelineChannel, false)
	alertChannel := make(chan *AlertInfo, 1024)
	log_ad := NewWatchdogLogAnomalyDetector(alertChannel, observationsChannel)
	log_ad.Alpha = 0.9
	log_ad.EvictionThreshold = 0.1 // TODO: Test that
	log_ad.PreprocessLen = 2
	log_ad.EvalLen = 1
	log_ad.BaselineLen = 1
	// Send only one alert per group
	log_ad.AlertCooldown = 999 * time.Hour

	// Init processor (do a shallow init to use the custom pipeline)
	processor := &PatternLogProcessor{ClustererPipeline: pipeline, AnomalyDetectors: []LogAnomalyDetector{log_ad}}
	log_ad.SetProcessor(processor)

	// Create batches
	batches := [][]*LogAnomalyDetectionProcessInput{}
	for _, batch := range logs {
		currBatch := []*LogAnomalyDetectionProcessInput{}
		for _, log := range batch {
			tags := ParseTags(log.GetTags())
			pipeline.Process(&patterns.TokenizerInput[*LogADTags]{Message: string(log.GetContent()), UserData: &tags})
			select {
			case result := <-pipelineChannel:
				currBatch = append(currBatch, &LogAnomalyDetectionProcessInput{ClustererInput: result.ClusterInput, ClustererResult: result.ClusterResult})
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("Timeout waiting for result")
			}
		}
		batches = append(batches, currBatch)
	}

	// Do anomaly detection batch by batch
	for _, batch := range batches {
		log_ad.ProcessBatch(batch)
		// TODO: Verify alerts here
	}

	// Verify keys
	// 2 patterns, 2 different set of tags -> 3 group by keys
	assert.Equal(t, len(log_ad.GroupByKeys), 3)

	// Verify alerts
	alerts := []AlertInfo{}
	close(alertChannel)
	for alert := range alertChannel {
		alerts = append(alerts, *alert)
	}
	for _, alert := range alerts {
		fmt.Printf("Alert: %s\n", alert.Describe())
	}
}
*/

func TestLogADAllTags(t *testing.T) {
	// Ensure sorted
	sortedTags := make([]string, len(logADAllTags))
	copy(sortedTags, logADAllTags[:])
	sort.Strings(sortedTags)
	assert.Equal(t, sortedTags, logADAllTags[:], "Tags should be sorted")
}

func TestParseTagsAndFullTags(t *testing.T) {
	// Test with no tags
	noTags := []string{}
	parsed := ParseTags(noTags)
	assert.Equal(t, [6]string{}, parsed.TagValues)
	assert.Empty(t, parsed.FullTags())

	// Test with unrelated tags (should be ignored)
	unrelatedTags := []string{"foo:bar", "another:tag"}
	parsed = ParseTags(unrelatedTags)
	assert.Equal(t, [6]string{}, parsed.TagValues)
	assert.Empty(t, parsed.FullTags())

	// Test with one tag
	tags := []string{"env:prod"}
	parsed = ParseTags(tags)
	expected := [6]string{}
	expected[1] = "prod"
	assert.Equal(t, expected, parsed.TagValues)
	assert.Equal(t, []string{"env:prod"}, parsed.FullTags())

	// Test with all tags in various orders
	all := []string{
		"dirname:/app/logs",
		"env:stage",
		"filename:main.go",
		"pod_name:nginx-123",
		"service:api",
		"source:k8s",
	}
	parsed = ParseTags(all)
	outTags := parsed.FullTags()
	sort.Strings(outTags)
	expectedTags := make([]string, len(logADAllTags))
	for i, tagName := range logADAllTags {
		expectedTags[i] = tagName + ":" + strings.TrimPrefix(all[i], tagName+":")
	}
	sort.Strings(expectedTags)
	assert.Equal(t, expectedTags, outTags)
}

func TestLogADTagsWriteHashConsistency(t *testing.T) {
	// Hashes should match for identical tags, even with different allocations
	tagsA := []string{"env:dev", "service:foo"}
	tagsB := []string{"service:foo", "env:dev"}
	parsedA := ParseTags(tagsA)
	parsedB := ParseTags(tagsB)
	var h1, h2 hash.Hash64
	h1 = fnv.New64()
	parsedA.WriteHash(h1)
	h2 = fnv.New64()
	parsedB.WriteHash(h2)
	assert.Equal(t, h1.Sum64(), h2.Sum64())

	// Hash should change if a tag value changes
	tagsC := []string{"env:prod", "service:foo"}
	parsedC := ParseTags(tagsC)
	var h3 hash.Hash64 = fnv.New64()
	parsedC.WriteHash(h3)
	assert.NotEqual(t, h1.Sum64(), h3.Sum64())
}

func TestLogADTagsFullTagsOmitsEmpty(t *testing.T) {
	tags := LogADTags{}
	// Set only some tags
	tags.TagValues[0] = "logfolder"
	tags.TagValues[3] = "pod-45"
	ftags := tags.FullTags()
	assert.Contains(t, ftags, "dirname:logfolder")
	assert.Contains(t, ftags, "pod_name:pod-45")
	assert.Len(t, ftags, 2)
}

func TestGroupByKeyHash(t *testing.T) {
	t.Run("same", func(t *testing.T) {
		key := &LogADGroupByKey{ClusterID: 1, Tags: &LogADTags{TagValues: [6]string{"test", "test", "test", "test", "test", "/test"}}}
		key2 := &LogADGroupByKey{ClusterID: 1, Tags: &LogADTags{TagValues: [6]string{"test", "test", "test", "test", "test", "/test"}}}
		hash := key.Hash()
		hash2 := key2.Hash()
		assert.Equal(t, hash, hash2)
	})

	t.Run("diff_cluster_id", func(t *testing.T) {
		key := &LogADGroupByKey{ClusterID: 1, Tags: &LogADTags{TagValues: [6]string{"test", "test", "test", "test", "test", "/test"}}}
		key2 := &LogADGroupByKey{ClusterID: 2, Tags: &LogADTags{TagValues: [6]string{"test", "test", "test", "test", "test", "/test"}}}
		hash := key.Hash()
		hash2 := key2.Hash()
		assert.NotEqual(t, hash, hash2)
	})

	t.Run("same_cluster_id", func(t *testing.T) {
		key := &LogADGroupByKey{ClusterID: 1, Tags: &LogADTags{TagValues: [6]string{"test", "test", "test", "test", "test", "/test"}}}
		key2 := &LogADGroupByKey{ClusterID: 1, Tags: &LogADTags{TagValues: [6]string{"test", "test", "test", "test", "test", "/var"}}}
		hash := key.Hash()
		hash2 := key2.Hash()
		assert.NotEqual(t, hash, hash2)
	})
}
