// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/jmoiron/sqlx"

	utilcommon "github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dataObservabilityWorkerState struct {
	dataObservabilityWorkerMu      sync.Mutex
	dataObservabilityWorkerRunning bool
	dataObservabilityWorkerStopped bool
	dataObservabilityWorkerDone    chan struct{}
	dataObservabilityWorkerCancel  context.CancelFunc
	dataObservabilityLastCheckRun  time.Time
	dataObservabilityLastEvent     time.Time
	dataObservabilityQueries       []*dataObservabilityScheduledQuery
	dataObservabilityDB            *sqlx.DB
	dataObservabilityConn          *sqlx.Conn
	dataObservabilityContainer     string
	dataObservabilityEmitter       func(dataObservabilityDueQuery, dataObservabilityQueryResult)
}

func (c *Check) runDataObservabilityJob(run func(context.Context) error) error {
	if !c.config.DataObservability.Enabled || len(c.dataObservabilityQueries) == 0 {
		return nil
	}
	c.dataObservabilityWorkerMu.Lock()
	if c.dataObservabilityWorkerStopped {
		c.dataObservabilityWorkerMu.Unlock()
		return nil
	}
	emitter, err := c.newDataObservabilityEmitter()
	if err != nil {
		c.dataObservabilityWorkerMu.Unlock()
		return err
	}
	c.dataObservabilityEmitter = emitter
	now := c.clock.Now()
	c.dataObservabilityLastCheckRun = now
	if c.dataObservabilityWorkerRunning {
		c.dataObservabilityWorkerMu.Unlock()
		return nil
	}
	interval := time.Duration(c.config.DataObservability.CollectionInterval * float64(time.Second))
	synchronous := c.config.DataObservability.RunSync
	if synchronous && !c.dataObservabilityLastEvent.IsZero() && now.Sub(c.dataObservabilityLastEvent) < interval {
		c.dataObservabilityWorkerMu.Unlock()
		return nil
	}
	parent, _ := utilcommon.GetMainCtxCancel()
	if parent.Err() != nil {
		c.dataObservabilityWorkerMu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	c.dataObservabilityWorkerRunning = true
	c.dataObservabilityWorkerCancel = cancel
	c.dataObservabilityWorkerDone = make(chan struct{})
	if synchronous {
		c.dataObservabilityLastEvent = now
	}
	runClock := c.clock
	inactivityTimeout := 2 * c.Interval()
	logPrompt := c.logPrompt
	c.dataObservabilityWorkerMu.Unlock()
	if synchronous {
		return c.runDataObservabilityWorker(ctx, runClock, interval, inactivityTimeout, true, run)
	}
	go func() {
		if err := c.runDataObservabilityWorker(ctx, runClock, interval, inactivityTimeout, false, run); err != nil && !errors.Is(err, context.Canceled) {
			log.Warnf("%s data observability job failed: %v", logPrompt, err)
		}
	}()
	return nil
}

func (c *Check) runDataObservabilityWorker(ctx context.Context, runClock clock.Clock, interval, inactivityTimeout time.Duration, synchronous bool, run func(context.Context) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("data observability job panic: %v", recovered)
			log.Warnf("%v\n%s", err, debug.Stack())
		}
		c.closeDataObservabilityConnection()
		c.dataObservabilityWorkerMu.Lock()
		c.dataObservabilityWorkerCancel()
		c.dataObservabilityWorkerCancel = nil
		c.dataObservabilityWorkerRunning = false
		close(c.dataObservabilityWorkerDone)
		c.dataObservabilityWorkerMu.Unlock()
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.dataObservabilityWorkerMu.Lock()
		inactive := runClock.Since(c.dataObservabilityLastCheckRun) > inactivityTimeout
		lastEvent := c.dataObservabilityLastEvent
		c.dataObservabilityWorkerMu.Unlock()
		if !synchronous && inactive {
			return nil
		}
		err := run(ctx)
		if synchronous {
			return err
		}
		// DBMAsyncJob rate-limits after each attempt, including failed attempts.
		if !lastEvent.IsZero() {
			if remaining := interval - runClock.Since(lastEvent); remaining > 0 && ctx.Err() == nil {
				timer := runClock.Timer(remaining)
				select {
				case <-timer.C:
				case <-ctx.Done():
				}
				timer.Stop()
			}
		}
		c.dataObservabilityWorkerMu.Lock()
		c.dataObservabilityLastEvent = runClock.Now()
		c.dataObservabilityWorkerMu.Unlock()
		if err != nil {
			return err
		}
	}
}

func (c *Check) stopDataObservabilityWorker(permanent bool) {
	if c.dataObservabilityWorkerState == nil {
		return
	}
	c.dataObservabilityWorkerMu.Lock()
	if permanent {
		c.dataObservabilityWorkerStopped = true
	}
	if c.dataObservabilityWorkerCancel != nil {
		c.dataObservabilityWorkerCancel()
	}
	done := c.dataObservabilityWorkerDone
	c.dataObservabilityWorkerMu.Unlock()
	if done != nil {
		<-done
	}
}
