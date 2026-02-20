// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package pipeline provides a concurrent processing pipeline for symbol upload operations.
package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"golang.org/x/sync/semaphore"
)

type Stage interface {
	Start(ctx context.Context)
	Stop()
}

type ConsumerWorker[In any] struct {
	wg             sync.WaitGroup
	inputChan      <-chan In
	concurrency    int
	processingFunc func(context.Context, In)
}

type StageWorker[In any, Out any] struct {
	ConsumerWorker[In]

	outputChan chan Out
}

type BatchingStageWorker[In any] struct {
	StageWorker[In, []In]

	batchSize     int
	batchInterval time.Duration
	clock         clockwork.Clock
}

// NewBudgetedProcessingFunc wraps a processing function with memory budget management.
// The wrapped function will automatically acquire budget before processing and release it after.
func NewBudgetedProcessingFunc[In any](budget int64, costCalculator func(In) int64, fun func(context.Context, In)) func(context.Context, In) {
	budgetSemaphore := semaphore.NewWeighted(budget)
	return func(ctx context.Context, i In) {
		cost := costCalculator(i)

		err := budgetSemaphore.Acquire(ctx, cost)
		if err != nil {
			return // the context is done
		}
		defer budgetSemaphore.Release(cost)

		fun(ctx, i)
	}
}

func newConsumerWorker[In any](inputChan <-chan In, concurrency int, fun func(context.Context, In)) ConsumerWorker[In] {
	return ConsumerWorker[In]{
		inputChan:      inputChan,
		concurrency:    concurrency,
		processingFunc: fun,
	}
}

func NewSinkStage[In any](inputChan <-chan In, fun func(context.Context, In), options ...StageOption) *ConsumerWorker[In] {
	opts := NewStageOptions(options...)
	w := newConsumerWorker(inputChan, opts.concurrency, fun)
	return &w
}

func NewStage[In any, Out any](inputChan <-chan In, fun func(context.Context, In, chan<- Out), options ...StageOption) *StageWorker[In, Out] {
	opts := NewStageOptions(options...)
	output := make(chan Out, opts.outputChanSize)
	return &StageWorker[In, Out]{
		ConsumerWorker: newConsumerWorker(inputChan, opts.concurrency, func(ctx context.Context, i In) {
			fun(ctx, i, output)
		}),
		outputChan: output,
	}
}

func NewBatchingStage[In any](inputChan <-chan In, batchInterval time.Duration, batchSize int, options ...StageOption) *BatchingStageWorker[In] {
	return NewBatchingStageWithClock[In](inputChan, batchInterval, batchSize, clockwork.NewRealClock(), options...)
}

func NewBatchingStageWithClock[In any](inputChan <-chan In, batchInterval time.Duration, batchSize int, clock clockwork.Clock, options ...StageOption) *BatchingStageWorker[In] {
	opts := NewStageOptions(options...)
	output := make(chan []In, opts.outputChanSize)
	return &BatchingStageWorker[In]{
		StageWorker: StageWorker[In, []In]{
			ConsumerWorker: newConsumerWorker(inputChan, opts.concurrency, nil),
			outputChan:     output,
		},

		batchSize:     batchSize,
		batchInterval: batchInterval,
		clock:         clock,
	}
}

func (w *ConsumerWorker[In]) Start(ctx context.Context) {
	for range w.concurrency {
		w.wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case input, ok := <-w.inputChan:
					if !ok {
						return
					}
					w.processingFunc(ctx, input)
				}
			}
		})
	}
}

func (w *ConsumerWorker[In]) Stop() {
	w.wg.Wait()
}

func (w *StageWorker[In, Out]) Stop() {
	w.ConsumerWorker.Stop()
	close(w.outputChan)
}

func (w *StageWorker[In, Out]) GetOutputChannel() <-chan Out {
	return w.outputChan
}

func (w *BatchingStageWorker[In]) Start(ctx context.Context) {
	for range w.concurrency {
		w.wg.Go(func() {
			var batch []In
			var tickerChan <-chan time.Time
			var ticker clockwork.Ticker
			if w.batchInterval > 0 {
				ticker = w.clock.NewTicker(w.batchInterval)
				tickerChan = ticker.Chan()
				defer ticker.Stop()
			}
			for {
				select {
				case <-ctx.Done():
					return
				case input, ok := <-w.inputChan:
					if !ok {
						if len(batch) > 0 {
							w.outputChan <- batch
						}
						return
					}
					batch = append(batch, input)
					if w.batchSize > 0 && len(batch) >= w.batchSize {
						if ticker != nil {
							ticker.Reset(w.batchInterval)
						}
						w.outputChan <- batch
						batch = nil
					}
				case <-tickerChan:
					if len(batch) > 0 {
						w.outputChan <- batch
						batch = nil
					}
				}
			}
		})
	}
}

type Pipeline[In any] struct {
	inputChan chan In
	workers   []Stage
}

func (p *Pipeline[In]) GetInputChannel() chan In {
	return p.inputChan
}
func (p *Pipeline[In]) Start(ctx context.Context) {
	for _, worker := range p.workers {
		worker.Start(ctx)
	}
}

func (p *Pipeline[In]) Stop() {
	close(p.inputChan)
	for _, worker := range p.workers {
		worker.Stop()
	}
}

func NewPipeline[In any](inputChan chan In, workers ...Stage) Pipeline[In] {
	return Pipeline[In]{
		inputChan: inputChan,
		workers:   workers,
	}
}

type StageOptions struct {
	concurrency    int
	outputChanSize int
}

type StageOption func(*StageOptions)

func WithConcurrency(concurrency int) StageOption {
	return func(o *StageOptions) {
		o.concurrency = concurrency
	}
}

func WithOutputChanSize(size int) StageOption {
	return func(o *StageOptions) {
		o.outputChanSize = size
	}
}

func NewStageOptions(options ...StageOption) StageOptions {
	o := StageOptions{
		concurrency:    1,
		outputChanSize: 0,
	}
	for _, option := range options {
		option(&o)
	}
	return o
}
