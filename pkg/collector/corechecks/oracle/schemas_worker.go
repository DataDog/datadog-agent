// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/semaphore"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	utilcommon "github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type schemaWorkerState struct {
	schemaWorkerMu      sync.Mutex
	schemaWorkerRunning bool
	schemaWorkerStopped bool
	schemaWorkerDone    chan struct{}
	schemaWorkerCancel  context.CancelFunc
}

func (c *Check) collectSchemasIfDue() error {
	if !c.config.Schemas.Enabled || (!c.dbmEnabled && !c.config.DataObservability.Enabled) {
		return nil
	}
	c.schemaWorkerMu.Lock()
	if c.schemaWorkerStopped || c.schemaWorkerRunning {
		c.schemaWorkerMu.Unlock()
		return nil
	}
	now := c.clock.Now()
	if !c.schemasLastRun.IsZero() && now.Sub(c.schemasLastRun) < time.Duration(c.config.Schemas.CollectionInterval)*time.Second {
		c.schemaWorkerMu.Unlock()
		return nil
	}
	parent, _ := utilcommon.GetMainCtxCancel()
	if parent.Err() != nil {
		c.schemaWorkerMu.Unlock()
		return nil
	}
	run, err := c.newSchemaCollectionRun()
	if err != nil {
		c.schemaWorkerMu.Unlock()
		return err
	}
	pool := c.db
	ctx, cancel := context.WithCancel(parent)
	c.schemaWorkerRunning = true
	c.schemaWorkerCancel = cancel
	c.schemaWorkerDone = make(chan struct{})
	c.schemasLastRun = now
	// Single shared connection in tracing mode.
	synchronous := c.config.AgentSQLTrace.Enabled
	c.schemaWorkerMu.Unlock()
	if synchronous {
		return c.runSchemaWorker(ctx, pool, run)
	}
	go func() {
		if err := c.runSchemaWorker(ctx, pool, run); err != nil {
			log.Warnf("%s failed to collect schemas: %v", run.logPrompt, err)
		}
	}()
	return nil
}

func (c *Check) runSchemaWorker(ctx context.Context, pool *sqlx.DB, run *Check) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("schema collection panic: %v", recovered)
			log.Warnf("%s %v\n%s", run.logPrompt, err, debug.Stack())
		}
		c.schemaWorkerMu.Lock()
		c.lastSnapshotID = run.lastSnapshotID
		c.schemaWorkerCancel()
		c.schemaWorkerCancel = nil
		c.schemaWorkerRunning = false
		close(c.schemaWorkerDone)
		c.schemaWorkerMu.Unlock()
	}()
	if pool == nil {
		return errors.New("schema connection pool is unavailable")
	}
	acquireCtx, cancel := context.WithTimeout(ctx, run.config.QueryTimeoutDuration())
	defer cancel()
	conn, err := pool.Connx(acquireCtx)
	if err != nil {
		return fmt.Errorf("acquire schema connection: %w", err)
	}
	cancel()
	defer conn.Close()
	run.schemaQueryer = conn
	if !c.config.AgentSQLTrace.Enabled {
		run.schemaPool = pool
	}
	return run.schemaCollection(ctx)
}

func (c *Check) stopSchemaWorker(permanent bool) {
	if c.schemaWorkerState == nil {
		return
	}
	c.schemaWorkerMu.Lock()
	if permanent {
		c.schemaWorkerStopped = true
	}
	if c.schemaWorkerCancel != nil {
		c.schemaWorkerCancel()
	}
	done := c.schemaWorkerDone
	c.schemaWorkerMu.Unlock()
	if done != nil {
		<-done
	}
}

// Cancel joins schema collection before the scheduler releases the sender.
func (c *Check) Cancel() {
	c.stopSchemaWorker(true)
}

func (c *Check) Stop() {
	c.stopSchemaWorker(true)
}

type schemaQueryer interface {
	QueryxContext(context.Context, string, ...interface{}) (*sqlx.Rows, error)
}

func (c *Check) schemaContextError(ctx context.Context) error {
	return errors.Join(ctx.Err(), c.schemaQueryError)
}

func (c *Check) newSchemaCollectionRun() (*Check, error) {
	sender, err := c.GetSender()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize schema sender: %w", err)
	}
	schemas := c.config.Schemas
	schemas.IncludeSchemas = slices.Clone(schemas.IncludeSchemas)
	schemas.ExcludeSchemas = slices.Clone(schemas.ExcludeSchemas)
	schemas.IncludeTables = slices.Clone(schemas.IncludeTables)
	schemas.ExcludeTables = slices.Clone(schemas.ExcludeTables)
	schemas.IncludeDatabases = slices.Clone(schemas.IncludeDatabases)
	schemas.ExcludeDatabases = slices.Clone(schemas.ExcludeDatabases)
	if schemas.CollectViews != nil {
		collectViews := *schemas.CollectViews
		schemas.CollectViews = &collectViews
	}
	return &Check{
		config: &config.CheckConfig{InstanceConfig: config.InstanceConfig{
			Schemas:          schemas,
			ConnectionConfig: config.ConnectionConfig{QueryTimeout: c.config.QueryTimeout},
		}},
		tags:                   slices.Clone(c.tags),
		driver:                 c.driver,
		cdbName:                c.cdbName,
		dbHostname:             c.dbHostname,
		dbInstanceIdentifier:   c.dbInstanceIdentifier,
		dbVersion:              c.dbVersion,
		agentVersion:           c.agentVersion,
		logPrompt:              c.logPrompt,
		clock:                  c.clock,
		schemaPayloadChunkSize: c.schemaPayloadChunkSize,
		lastSnapshotID:         c.lastSnapshotID,
		schemaEmitter: func(payload []byte) {
			sender.EventPlatformEvent(payload, "dbm-metadata")
		},
	}, nil
}

const (
	schemaWindowWorkers      = 4
	schemaWindowBufferBytes  = 4 << 20
	schemaWindowBufferEvents = 64
)

type schemaWindowEvent struct {
	done    bool
	encoded []byte
	credit  int64
}

func schemaOrderedWindows(ctx context.Context, count, workerCount int, process func(context.Context, int, int, func(schemaEvent) error) error, emit func(schemaEvent) error) error {
	return schemaOrderedWindowsBuffered(ctx, count, workerCount, schemaWindowBufferBytes, process, emit)
}

func schemaOrderedWindowsBuffered(ctx context.Context, count, workerCount int, bufferBytes int64, process func(context.Context, int, int, func(schemaEvent) error) error, emit func(schemaEvent) error) (err error) {
	if workerCount < 1 || workerCount > schemaWindowWorkers {
		return errors.New("invalid schema window worker count")
	}
	if bufferBytes <= 0 {
		return errors.New("schema window buffer must be positive")
	}
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	failures := make([]error, workerCount)
	output := make([]chan schemaWindowEvent, workerCount)
	// Separate allowances let the first worker progress even when later queues are full.
	credits := make([]*semaphore.Weighted, workerCount)
	defer func() {
		cancel()
		workers.Wait()
		for worker, queue := range output {
			for value := range queue {
				if value.credit > 0 {
					credits[worker].Release(value.credit)
				}
			}
		}
		err = errors.Join(err, errors.Join(failures...))
	}()
	for worker := range output {
		credits[worker] = semaphore.NewWeighted(bufferBytes)
		output[worker] = make(chan schemaWindowEvent, schemaWindowBufferEvents)
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			defer close(output[worker])
			defer func() {
				if recovered := recover(); recovered != nil {
					failures[worker] = fmt.Errorf("schema window worker panic: %v", recovered)
				}
				if failures[worker] != nil {
					cancel()
				}
			}()
			send := func(value schemaWindowEvent) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				select {
				case output[worker] <- value:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			for window := worker; window < count; window += len(output) {
				if err := ctx.Err(); err != nil {
					failures[worker] = err
					return
				}
				if err := process(ctx, worker, window, func(event schemaEvent) error {
					encoded, err := json.Marshal(event)
					if err != nil {
						return err
					}
					// Allow oversized events through to avoid blocking their worker forever.
					credit := min(int64(cap(encoded)), bufferBytes)
					if err := credits[worker].Acquire(ctx, credit); err != nil {
						return err
					}
					if err := send(schemaWindowEvent{encoded: encoded, credit: credit}); err != nil {
						credits[worker].Release(credit)
						return err
					}
					return nil
				}); err != nil {
					failures[worker] = err
					return
				}
				if err := send(schemaWindowEvent{done: true}); err != nil {
					failures[worker] = err
					return
				}
			}
		}(worker)
	}
	for window := 0; window < count; window++ {
		for {
			select {
			case value, ok := <-output[window%len(output)]:
				if !ok {
					return errors.New("schema window ended before completion")
				}
				if value.done {
					goto nextWindow
				}
				if err := ctx.Err(); err != nil {
					if value.credit > 0 {
						credits[window%len(output)].Release(value.credit)
					}
					return err
				}
				if err := func() error {
					defer credits[window%len(output)].Release(value.credit)
					var event schemaEvent
					if err := json.Unmarshal(value.encoded, &event); err != nil {
						return err
					}
					return emit(event)
				}(); err != nil {
					return err
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	nextWindow:
	}
	return nil
}

func (c *Check) schemaWindowCheck(queryer schemaQueryer) *Check {
	return &Check{
		config:                 c.config,
		driver:                 c.driver,
		tags:                   c.tags,
		cdbName:                c.cdbName,
		dbHostname:             c.dbHostname,
		dbInstanceIdentifier:   c.dbInstanceIdentifier,
		dbVersion:              c.dbVersion,
		agentVersion:           c.agentVersion,
		logPrompt:              c.logPrompt,
		clock:                  c.clock,
		schemaPayloadChunkSize: c.schemaPayloadChunkSize,
		schemaQueryer:          queryer,
	}
}

func (c *Check) schemaTableWindows(ctx context.Context, keys []tableKey, selected map[tableKey]struct{}, owners map[ownerKey]string, container map[int64]string, truncated map[int64]struct{}, emit func(schemaEvent) error) (int, error) {
	workerCount := 1
	if c.schemaPool != nil {
		workerCount = min(schemaWindowWorkers, (len(keys)+schemaConstraintPageSize-1)/schemaConstraintPageSize)
	}
	if workerCount == 1 || len(keys) <= schemaConstraintPageSize {
		total := 0
		err := forEachTablePage(keys, schemaConstraintPageSize, func(window []tableKey) error {
			count, err := c.schemaTableWindow(ctx, window, selected, owners, container, truncated, emit)
			total += count
			return err
		})
		return total, err
	}
	queryers := []schemaQueryer{c.schemaQueryer}
	for len(queryers) < workerCount {
		acquireCtx, cancel := context.WithTimeout(ctx, c.config.QueryTimeoutDuration())
		conn, err := c.schemaPool.Connx(acquireCtx)
		cancel()
		if err != nil {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			break
		}
		defer conn.Close()
		queryers = append(queryers, conn)
	}
	return c.schemaTableWindowsOnConnections(ctx, keys, selected, owners, container, truncated, queryers, emit)
}

func (c *Check) schemaTableWindowsOnConnections(ctx context.Context, keys []tableKey, selected map[tableKey]struct{}, owners map[ownerKey]string, container map[int64]string, truncated map[int64]struct{}, queryers []schemaQueryer, emit func(schemaEvent) error) (int, error) {
	workers := make([]*Check, len(queryers))
	for i, queryer := range queryers {
		workers[i] = c.schemaWindowCheck(queryer)
	}
	var windows [][]tableKey
	if err := forEachTablePage(keys, schemaConstraintPageSize, func(window []tableKey) error {
		windows = append(windows, window)
		return nil
	}); err != nil {
		return 0, err
	}
	totals := make([]int, len(workers))
	err := schemaOrderedWindows(ctx, len(windows), len(workers), func(ctx context.Context, worker, window int, emit func(schemaEvent) error) error {
		count, err := workers[worker].schemaTableWindow(ctx, windows[window], selected, owners, container, truncated, emit)
		totals[worker] += count
		return err
	}, emit)
	total := 0
	for _, count := range totals {
		total += count
	}
	return total, err
}
