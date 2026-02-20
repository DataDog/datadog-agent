// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

// Used to efficiently cluster logs in a multi-threaded way

import (
	"fmt"
	"math/rand"
	"strings"
)

type MultiThreadPipeline struct {
	NCpus             int
	Tokenizers        []*MultiThreadTokenizer
	PatternClusterers []*MultiThreadPatternClusterer
	// Will send the process results here
	ResultChannel chan *MultiThreadResult
	OnlyTokenize  bool
}

type MultiThreadResult struct {
	ClusterInput  *ClustererInput
	ClusterResult *ClusterResult
}

type ClustererInput struct {
	Tokens  []Token
	Message string
}

type MultiThreadTokenizer struct {
	Pipeline  *MultiThreadPipeline
	ID        int
	Tokenizer *Tokenizer
	// Where we send the tokens to be tokenized
	Channel chan string
}

type MultiThreadPatternClusterer struct {
	Pipeline *MultiThreadPipeline
	ID       int
	// Where we send the tokens to be clustered
	Channel          chan *ClustererInput
	PatternClusterer *PatternClusterer
}

func NewMultiThreadPipeline(nCpus int, resultChannel chan *MultiThreadResult, onlyTokenize bool) *MultiThreadPipeline {
	pipeline := &MultiThreadPipeline{
		NCpus:             nCpus,
		OnlyTokenize:      onlyTokenize,
		ResultChannel:     resultChannel,
		Tokenizers:        make([]*MultiThreadTokenizer, nCpus),
		PatternClusterers: make([]*MultiThreadPatternClusterer, nCpus),
	}

	for i := 0; i < nCpus; i++ {
		pipeline.Tokenizers[i] = &MultiThreadTokenizer{Pipeline: pipeline, ID: i, Tokenizer: NewTokenizer(), Channel: make(chan string, 1024)}
		go pipeline.Tokenizers[i].Run()

		pipeline.PatternClusterers[i] = &MultiThreadPatternClusterer{Pipeline: pipeline, ID: i, PatternClusterer: NewPatternClusterer(IDComputeInfo{Offset: i, Stride: nCpus, Index: 0}), Channel: make(chan *ClustererInput, 1024)}
		go pipeline.PatternClusterers[i].Run()
	}

	return pipeline
}

func (pipeline *MultiThreadPipeline) Process(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	tokenizerId := rand.Intn(pipeline.NCpus)
	pipeline.Tokenizers[tokenizerId].Channel <- message
}

func (pipeline *MultiThreadPipeline) GetClusterInfo(patternID int) (ClusterInfo, error) {
	// TODO: We can't use the string repr since it's not thread safe, we should block this when necessary (once the anomaly is detected)
	for _, clusterer := range pipeline.PatternClusterers {
		for _, cluster := range clusterer.PatternClusterer.GetClusters() {
			if cluster.ID == patternID {
				return cluster.ToClusterInfo(), nil
			}
		}
	}

	return ClusterInfo{}, fmt.Errorf("cluster not found")
}

func (tokenizer *MultiThreadTokenizer) Run() {
	for message := range tokenizer.Channel {
		tokens := tokenizer.Tokenizer.Tokenize(message)
		if tokenizer.Pipeline.OnlyTokenize {
			tokenizer.Pipeline.ResultChannel <- &MultiThreadResult{ClusterInput: &ClustererInput{Tokens: tokens, Message: message}}
			continue
		}
		patternMatcherCpu := len(tokens) % tokenizer.Pipeline.NCpus
		tokenizer.Pipeline.PatternClusterers[patternMatcherCpu].Channel <- &ClustererInput{Tokens: tokens, Message: message}
	}
}

// Event loop to listen for new tokens
func (clusterer *MultiThreadPatternClusterer) Run() {
	for input := range clusterer.Channel {
		result := clusterer.PatternClusterer.ProcessTokens(input.Tokens, input.Message)
		clusterer.Pipeline.ResultChannel <- &MultiThreadResult{ClusterInput: input, ClusterResult: result}
	}
}
