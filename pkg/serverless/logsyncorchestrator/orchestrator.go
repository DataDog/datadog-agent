// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logsyncorchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
)

const MAX_RETRY_COUNT = 20

type LogSyncOrchestrator struct {
	isProcessing                     bool
	TelemetryApiMessageReceivedCount atomic.Uint32
	nbMessageSent                    atomic.Uint32
	wg                               *sync.WaitGroup
}

func NewLogSyncOrchestrator() *LogSyncOrchestrator {
	return &LogSyncOrchestrator{
		wg: &sync.WaitGroup{},
	}
}

func (l *LogSyncOrchestrator) WaitIncomingRequest() {
	l.wg.Wait()
}

func (l *LogSyncOrchestrator) AllowIncomingRequest() {
	l.wg.Done()
}

func (l *LogSyncOrchestrator) BlockIncomingRequest() {
	l.wg.Add(1)
}

func (l *LogSyncOrchestrator) Wait(retryIdx int, ctx context.Context, flush func(ctx context.Context)) {
	if retryIdx > MAX_RETRY_COUNT {
		log.Error("logSync LogSyncOrchestrator.Wait() failed, retryIdx > 20 (2)")
	} else {
		receivedCount := l.TelemetryApiMessageReceivedCount.Load()
		sent := l.nbMessageSent.Load()
		if receivedCount != sent {
			log.Debugf("logSync needs to wait (%v/%v)\n", receivedCount, sent)
			time.Sleep(100 * time.Millisecond)
			if !l.isProcessing {
				log.Debugf("logSync is not processing, trigger a new flush\n")
				flush(ctx)
			} else {
				log.Debugf("logSync is processing, just wait\n")
			}
			l.Wait(retryIdx+1, ctx, flush)
		} else {
			log.Debug("logSync is balanced")
		}
	}
}

func (l *LogSyncOrchestrator) Add(nbMessageReceived uint32) {
	l.nbMessageSent.Add(nbMessageReceived)
}

func (l *LogSyncOrchestrator) Processing(isProcessing bool) {
	l.isProcessing = isProcessing
}
