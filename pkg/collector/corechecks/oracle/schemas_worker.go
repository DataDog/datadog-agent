// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

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

// Stop cancels schema collection when the runner stops the check.
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
