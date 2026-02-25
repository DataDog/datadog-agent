// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type TestUserData struct {
	ID int
}

func TestMultiThreadPipeline(t *testing.T) {
	const nCpus = 4
	pipeline := NewMultiThreadPipeline(nCpus, make(chan *MultiThreadResult[*TestUserData], 1024), false)
	assert.Equal(t, nCpus, len(pipeline.PatternClusterers))

	// Results without empty messages
	const expectedResults = 5
	pipeline.Process(&TokenizerInput[*TestUserData]{Message: "", UserData: &TestUserData{ID: 0}})
	pipeline.Process(&TokenizerInput[*TestUserData]{Message: "this is my test 1", UserData: &TestUserData{ID: 1}})
	pipeline.Process(&TokenizerInput[*TestUserData]{Message: "hello beautiful world", UserData: &TestUserData{ID: 10}})
	pipeline.Process(&TokenizerInput[*TestUserData]{Message: "this is my test 2", UserData: &TestUserData{ID: 2}})
	pipeline.Process(&TokenizerInput[*TestUserData]{Message: "     ", UserData: &TestUserData{ID: 100}})
	pipeline.Process(&TokenizerInput[*TestUserData]{Message: "r u vibecoder?", UserData: &TestUserData{ID: 20}})
	pipeline.Process(&TokenizerInput[*TestUserData]{Message: "this is my test 3", UserData: &TestUserData{ID: 3}})

	// Wait for all results with timeout
	results := make([]*MultiThreadResult[*TestUserData], expectedResults)
	for i := 0; i < expectedResults; i++ {
		select {
		case results[i] = <-pipeline.ResultChannel:
			// got result
		case <-time.After(1 * time.Second):
			t.Fatalf("Timeout waiting for result %d/%d", i+1, expectedResults)
		}
	}

	// Sort results by user data ID
	sort.Slice(results, func(i, j int) bool {
		return results[i].ClusterInput.UserData.ID < results[j].ClusterInput.UserData.ID
	})

	// Test corresponding user data
	assert.Equal(t, []int{1, 2, 3, 10, 20}, []int{results[0].ClusterInput.UserData.ID, results[1].ClusterInput.UserData.ID, results[2].ClusterInput.UserData.ID, results[3].ClusterInput.UserData.ID, results[4].ClusterInput.UserData.ID})
	assert.Equal(t, []string{"this is my test 1", "this is my test 2", "this is my test 3", "hello beautiful world", "r u vibecoder?"}, []string{results[0].ClusterInput.Message, results[1].ClusterInput.Message, results[2].ClusterInput.Message, results[3].ClusterInput.Message, results[4].ClusterInput.Message})

	// Test dispatched to correct clusterer
	// this is my test * -> 9 % nCpus = 1
	// hello beautiful world -> 1
	// r u vibecoder? -> 2
	assert.Equal(t, 0, pipeline.PatternClusterers[0].PatternClusterer.NumClusters())
	assert.Equal(t, 2, pipeline.PatternClusterers[1].PatternClusterer.NumClusters())
	assert.Equal(t, 1, pipeline.PatternClusterers[2].PatternClusterer.NumClusters())
	assert.Equal(t, 0, pipeline.PatternClusterers[3].PatternClusterer.NumClusters())

	assert.Contains(t, pipeline.PatternClusterers[1].PatternClusterer.GetClusters()[0].PatternString(), "this is my test *")
	assert.Contains(t, pipeline.PatternClusterers[1].PatternClusterer.GetClusters()[1].PatternString(), "hello beautiful world")
	assert.Equal(t, "r u vibecoder?", pipeline.PatternClusterers[2].PatternClusterer.GetClusters()[0].PatternString())
}
