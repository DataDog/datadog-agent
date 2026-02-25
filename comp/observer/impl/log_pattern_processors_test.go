// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestGroupByKeyHash(t *testing.T) {
	t.Run("same", func(t *testing.T) {
		key := &LogADGroupByKey{ClusterID: 1, Tags: &LogADTags{Env: "test", PodName: "test", Service: "test", Source: "test", DirName: "/test"}}
		key2 := &LogADGroupByKey{ClusterID: 1, Tags: &LogADTags{Env: "test", PodName: "test", Service: "test", Source: "test", DirName: "/test"}}
		hash := key.Hash()
		hash2 := key2.Hash()
		assert.Equal(t, hash, hash2)
	})

	t.Run("diff_cluster_id", func(t *testing.T) {
		key := &LogADGroupByKey{ClusterID: 1, Tags: &LogADTags{Env: "test", PodName: "test", Service: "test", Source: "test", DirName: "/test"}}
		key2 := &LogADGroupByKey{ClusterID: 2, Tags: &LogADTags{Env: "test", PodName: "test", Service: "test", Source: "test", DirName: "/test"}}
		hash := key.Hash()
		hash2 := key2.Hash()
		assert.NotEqual(t, hash, hash2)
	})

	t.Run("same_cluster_id", func(t *testing.T) {
		key := &LogADGroupByKey{ClusterID: 1, Tags: &LogADTags{Env: "test", PodName: "test", Service: "test", Source: "test", DirName: "/test"}}
		key2 := &LogADGroupByKey{ClusterID: 1, Tags: &LogADTags{Env: "test", PodName: "test", Service: "test", Source: "test", DirName: "/var"}}
		hash := key.Hash()
		hash2 := key2.Hash()
		assert.NotEqual(t, hash, hash2)
	})
}

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
	// log_ad.AlertCooldown = 0 * time.Second

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

// func TestWatchdogLogAnomalyDetector(t *testing.T) {
// 	observationsChannel := make(chan observation, 1024)
// 	logs := []*message.Message{
// 		message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 200 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("this is my test 2"), nil, "info", time.Now().Unix()),
// 		// We use nil to simulate a time step
// 		nil,
// 		message.NewMessage([]byte("this is my test 0"), nil, "info", time.Now().Unix()),
// 		nil,
// 		message.NewMessage([]byte("this is my test 0"), nil, "info", time.Now().Unix()),
// 		nil,
// 		message.NewMessage([]byte("this is my test 0"), nil, "info", time.Now().Unix()),
// 		nil,
// 		message.NewMessage([]byte("this is my test 0"), nil, "info", time.Now().Unix()),
// 		nil,
// 		message.NewMessage([]byte("this is my test 0"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 200 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		nil,
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
// 		nil,
// 		message.NewMessage([]byte("http 200 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		nil,
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 200 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("http 404 on /api/v1/users"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
// 		message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
// 		nil,
// 		message.NewMessage([]byte("this is my test 1"), nil, "info", time.Now().Unix()),
// 	}

// 	const nCpus = 4
// 	pipelineChannel := make(chan *patterns.MultiThreadResult[*LogADTags], 1024)
// 	pipeline := patterns.NewMultiThreadPipeline[*LogADTags](nCpus, pipelineChannel, false)
// 	resultChannel := make(chan *observer.LogProcessorResult, 1024)
// 	log_ad := NewWatchdogLogAnomalyDetector(resultChannel, observationsChannel)

// 	// TODO A: How to manage time updates? -> We need to do it for the replay anyway
// 	for _, log := range logs {
// 		tags := ParseTags(log.GetTags())
// 		pipeline.Process(&patterns.TokenizerInput[*LogADTags]{Message: string(log.GetContent()), UserData: &tags})
// 		if log == nil {
// 			// TODO: Don't do that
// 			time.Sleep(100 * time.Millisecond)
// 			log_ad.Snapshot()
// 		}
// 	}

// 	// TODO: Process results
// 	for result := range pipelineChannel {
// 		fmt.Printf("Result: %+v\n", result)
// 	}
// }
