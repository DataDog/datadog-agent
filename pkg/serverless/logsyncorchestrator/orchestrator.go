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
	TelemetryApiMessageReceivedCount atomic.Uint32
	NbMessageSent                    atomic.Uint32
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

func (l *LogSyncOrchestrator) Wait(retryIdx int, ctx context.Context, logsFlushMutex *sync.Mutex, flush func(ctx context.Context)) {
	if retryIdx > MAX_RETRY_COUNT {
		log.Error("LogSyncOrchestrator.Wait() failed, retryIdx > 20 (2)")
	} else {
		receivedCount := l.TelemetryApiMessageReceivedCount.Load()
		sent := l.NbMessageSent.Load()
		if receivedCount != sent {
			log.Debugf("logSync needs to wait (%v/%v)\n", receivedCount, sent)
			logsFlushMutex.Lock()
			flush(ctx)
			logsFlushMutex.Unlock()
			time.Sleep(100 * time.Millisecond)
			l.Wait(retryIdx+1, ctx, logsFlushMutex, flush)
		} else {
			log.Debug("logSync is balanced")
		}
	}
}
