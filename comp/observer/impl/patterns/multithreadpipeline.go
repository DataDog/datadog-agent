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

type MultiThreadPipeline[T any] struct {
	NCpus             int
	Tokenizers        []*MultiThreadTokenizer[T]
	PatternClusterers []*MultiThreadPatternClusterer[T]
	// Will send the process results here
	ResultChannel chan *MultiThreadResult[T]
	OnlyTokenize  bool
}

type MultiThreadResult[T any] struct {
	ClusterInput  *ClustererInput[T]
	ClusterResult *ClusterResult
}

type ClustererInput[T any] struct {
	Tokens  []Token
	Message string
	// This is not directly processed but used after in the pipeline
	UserData T
}

type MultiThreadTokenizer[T any] struct {
	Pipeline  *MultiThreadPipeline[T]
	ID        int
	Tokenizer *Tokenizer
	// Where we send the tokens to be tokenized
	Channel chan *TokenizerInput[T]
}

type TokenizerInput[T any] struct {
	Message  string
	UserData T
}

type MultiThreadPatternClusterer[T any] struct {
	Pipeline *MultiThreadPipeline[T]
	ID       int
	// Where we send the tokens to be clustered
	Channel          chan *ClustererInput[T]
	PatternClusterer *PatternClusterer
}

func NewMultiThreadPipeline[T any](nCpus int, resultChannel chan *MultiThreadResult[T], onlyTokenize bool) *MultiThreadPipeline[T] {
	pipeline := &MultiThreadPipeline[T]{
		NCpus:             nCpus,
		OnlyTokenize:      onlyTokenize,
		ResultChannel:     resultChannel,
		Tokenizers:        make([]*MultiThreadTokenizer[T], nCpus),
		PatternClusterers: make([]*MultiThreadPatternClusterer[T], nCpus),
	}

	for i := 0; i < nCpus; i++ {
		pipeline.Tokenizers[i] = &MultiThreadTokenizer[T]{Pipeline: pipeline, ID: i, Tokenizer: NewTokenizer(), Channel: make(chan *TokenizerInput[T], 1024)}
		go pipeline.Tokenizers[i].Run()

		pipeline.PatternClusterers[i] = &MultiThreadPatternClusterer[T]{Pipeline: pipeline, ID: i, PatternClusterer: NewPatternClusterer(IDComputeInfo{Offset: i, Stride: nCpus, Index: 0}), Channel: make(chan *ClustererInput[T], 1024)}
		go pipeline.PatternClusterers[i].Run()
	}

	return pipeline
}

func (pipeline *MultiThreadPipeline[T]) Process(tokInput *TokenizerInput[T]) {
	tokInput.Message = strings.TrimSpace(tokInput.Message)
	if tokInput.Message == "" {
		return
	}
	tokenizerId := rand.Intn(pipeline.NCpus)
	pipeline.Tokenizers[tokenizerId].Channel <- tokInput
}

func (pipeline *MultiThreadPipeline[T]) GetClusterInfo(patternID int) (ClusterInfo, error) {
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

func (tokenizer *MultiThreadTokenizer[T]) Run() {
	for tokInput := range tokenizer.Channel {
		tokens := tokenizer.Tokenizer.Tokenize(tokInput.Message)
		if tokenizer.Pipeline.OnlyTokenize {
			tokenizer.Pipeline.ResultChannel <- &MultiThreadResult[T]{ClusterInput: &ClustererInput[T]{Tokens: tokens, Message: tokInput.Message, UserData: tokInput.UserData}}
			continue
		}
		patternMatcherCpu := len(tokens) % tokenizer.Pipeline.NCpus
		tokenizer.Pipeline.PatternClusterers[patternMatcherCpu].Channel <- &ClustererInput[T]{Tokens: tokens, Message: tokInput.Message, UserData: tokInput.UserData}
	}
}

// Event loop to listen for new tokens
func (clusterer *MultiThreadPatternClusterer[T]) Run() {
	for input := range clusterer.Channel {
		result := clusterer.PatternClusterer.ProcessTokens(input.Tokens, input.Message)
		clusterer.Pipeline.ResultChannel <- &MultiThreadResult[T]{ClusterInput: input, ClusterResult: result}
	}
}
