// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/godror/godror"
	"github.com/jmoiron/sqlx"
	"github.com/robfig/cron/v3"
	"github.com/sijms/go-ora/v2/network"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const dataObservabilityCronStartupLookback = 300 * time.Second

const dataObservabilityMaxResultRows = 10_000

const dataObservabilityCurrentContainerQuery = `SELECT sys_context('USERENV', 'CON_NAME') FROM dual`

const dataObservabilityContainersQuery = `SELECT name FROM v$containers
WHERE open_mode IN ('READ WRITE', 'READ ONLY') AND NVL(restricted, 'NO') = 'NO'`

type dataObservabilityQueryResult struct {
	Status   string   `json:"status"`
	Columns  []string `json:"columns"`
	Rows     [][]any  `json:"rows"`
	RowCount int      `json:"row_count"`
	Duration float64  `json:"duration_s"`
	Error    *string  `json:"error"`
}

type dataObservabilityScheduledQuery struct {
	query           config.DataObservabilityQuery
	schedule        cron.Schedule
	cronInitialized bool
	nextTick        time.Time
	lastExecution   time.Time
	pendingRetry    *dataObservabilityDueQuery
}

type dataObservabilityDueQuery struct {
	scheduledQuery *dataObservabilityScheduledQuery
	scheduledTime  time.Time
}

func newDataObservabilityScheduledQueries(queries []config.DataObservabilityQuery) []*dataObservabilityScheduledQuery {
	var scheduled []*dataObservabilityScheduledQuery
	for _, query := range queries {
		entry := &dataObservabilityScheduledQuery{query: query}
		if query.Schedule != "" {
			schedule, err := cron.ParseStandard(query.Schedule)
			if err != nil {
				log.Warnf("Skipping DO query monitor_id=%d: invalid cron schedule %q: %v", query.MonitorID, query.Schedule, err)
				continue
			}
			entry.schedule = schedule
		}
		scheduled = append(scheduled, entry)
	}
	return scheduled
}

func getDueDataObservabilityQueries(queries []*dataObservabilityScheduledQuery, now time.Time) []dataObservabilityDueQuery {
	var due []dataObservabilityDueQuery
	for _, entry := range queries {
		var scheduled time.Time
		if entry.schedule != nil {
			// Python includes a tick exactly at poll time by adding one millisecond.
			cronNow := now.UTC().Add(time.Millisecond)
			firstPoll := !entry.cronInitialized
			if firstPoll {
				// Next is exclusive, but the startup lookback's lower bound is inclusive.
				entry.nextTick = entry.schedule.Next(cronNow.Add(-dataObservabilityCronStartupLookback).Add(-time.Nanosecond))
				entry.cronInitialized = true
			}
			for !entry.nextTick.IsZero() && !entry.nextTick.After(cronNow) {
				if !firstPoll || entry.nextTick.Before(cronNow) {
					scheduled = entry.nextTick
				}
				entry.nextTick = entry.schedule.Next(entry.nextTick)
			}
		} else {
			interval := time.Duration(entry.query.IntervalSeconds) * time.Second
			if entry.lastExecution.IsZero() {
				scheduled = now
			} else if now.Sub(entry.lastExecution) >= interval {
				scheduled = entry.lastExecution.Add(interval)
			}
		}
		if !scheduled.IsZero() {
			due = append(due, dataObservabilityDueQuery{scheduledQuery: entry, scheduledTime: scheduled})
		} else if entry.pendingRetry != nil {
			due = append(due, *entry.pendingRetry)
		}
		entry.pendingRetry = nil
	}
	return due
}

func (due dataObservabilityDueQuery) complete(now time.Time) {
	if due.scheduledQuery.schedule == nil {
		due.scheduledQuery.lastExecution = now
	}
}

func (due dataObservabilityDueQuery) retry() {
	due.scheduledQuery.pendingRetry = &due
}

func (c *Check) collectDataObservability(ctx context.Context) error {
	dueQueries := getDueDataObservabilityQueries(c.dataObservabilityQueries, c.clock.Now())
	if len(dueQueries) == 0 {
		return nil
	}
	slices.SortStableFunc(dueQueries, func(a, b dataObservabilityDueQuery) int {
		if order := cmp.Compare(a.scheduledQuery.query.DBName, b.scheduledQuery.query.DBName); order != 0 {
			return order
		}
		return cmp.Compare(a.scheduledQuery.query.QueryTimeout, b.scheduledQuery.query.QueryTimeout)
	})
	var containers map[string]string
	for i, due := range dueQueries {
		if i == 0 || c.dataObservabilityConn == nil {
			err := c.connectDataObservability(ctx)
			if err == nil {
				containers, err = c.dataObservabilityContainers(ctx)
			}
			if err != nil {
				c.closeDataObservabilityConnection()
				for _, pending := range dueQueries[i:] {
					pending.retry()
				}
				return err
			}
		}
		query := due.scheduledQuery.query
		result, err := c.executeDataObservabilityQuery(ctx, query, containers[query.DBName])
		if ctx.Err() != nil || (isDataObservabilityConnectionError(err) && !errors.Is(err, context.DeadlineExceeded)) {
			c.closeDataObservabilityConnection()
			for _, pending := range dueQueries[i:] {
				pending.retry()
			}
			return errors.Join(ctx.Err(), err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			c.closeDataObservabilityConnection()
		}
		due.complete(c.clock.Now())
		if err != nil {
			log.Warnf("Query failed for monitor_id=%d (%.3fs): %v | SQL: %s", query.MonitorID, result.Duration, err, query.Query)
		}
		if c.dataObservabilityEmitter != nil {
			c.dataObservabilityEmitter(due, result)
		}
	}
	return nil
}

func (c *Check) connectDataObservability(ctx context.Context) error {
	if c.dataObservabilityConn != nil {
		return nil
	}
	if c.dataObservabilityDB == nil {
		timeout := c.config.QueryTimeoutDuration()
		for _, entry := range c.dataObservabilityQueries {
			timeout = max(timeout, entry.query.QueryTimeoutDuration())
		}
		queryTimeout := strconv.FormatInt(int64((timeout-1)/time.Second)+1, 10)
		oracleDriver, connStr := c.connectionString(queryTimeout)
		db, err := sqlx.Open(oracleDriver, connStr)
		if err != nil {
			return fmt.Errorf("open data observability connection: %w", err)
		}
		db.SetMaxOpenConns(1)
		c.dataObservabilityDB = db
	}
	acquireCtx, cancel := context.WithTimeout(ctx, c.config.QueryTimeoutDuration())
	defer cancel()
	conn, err := c.dataObservabilityDB.Connx(acquireCtx)
	if err != nil {
		c.closeDataObservabilityConnection()
		return fmt.Errorf("acquire data observability connection: %w", err)
	}
	c.dataObservabilityConn = conn
	if err := conn.PingContext(acquireCtx); err != nil {
		c.closeDataObservabilityConnection()
		return fmt.Errorf("ping data observability connection: %w", err)
	}
	if err := conn.GetContext(acquireCtx, &c.dataObservabilityContainer, dataObservabilityCurrentContainerQuery); err != nil {
		c.closeDataObservabilityConnection()
		return fmt.Errorf("identify data observability container: %w", err)
	}
	return nil
}

func (c *Check) closeDataObservabilityConnection() {
	if c.dataObservabilityConn != nil {
		if err := c.dataObservabilityConn.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			log.Warnf("%s failed to close data observability session: %v", c.logPrompt, err)
		}
		c.dataObservabilityConn = nil
	}
	closeDatabase(c, c.dataObservabilityDB)
	c.dataObservabilityDB = nil
	c.dataObservabilityContainer = ""
}

func (c *Check) dataObservabilityContainers(ctx context.Context) (map[string]string, error) {
	containers := make(map[string]string)
	if !c.multitenant || c.connectedToPdb {
		containers[c.getFullPDBName(c.dataObservabilityContainer)] = c.dataObservabilityContainer
		return containers, nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, c.config.QueryTimeoutDuration())
	defer cancel()
	if _, err := c.dataObservabilityConn.ExecContext(queryCtx, dataObservabilitySetContainerSQL(c.dataObservabilityContainer)); err != nil {
		return nil, fmt.Errorf("restore data observability container: %w", err)
	}
	rows, err := c.dataObservabilityConn.QueryxContext(queryCtx, dataObservabilityContainersQuery)
	if err != nil {
		return nil, fmt.Errorf("discover data observability containers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		containers[c.getFullPDBName(name)] = name
	}
	return containers, rows.Err()
}

func dataObservabilitySetContainerSQL(container string) string {
	return fmt.Sprintf(`ALTER SESSION SET CONTAINER = "%s"`, strings.ReplaceAll(container, `"`, `""`))
}

func (c *Check) executeDataObservabilityQuery(ctx context.Context, query config.DataObservabilityQuery, container string) (result dataObservabilityQueryResult, err error) {
	start := c.clock.Now()
	result.Columns = []string{}
	result.Rows = [][]any{}
	queryCtx, cancel := context.WithTimeout(ctx, query.QueryTimeoutDuration())
	defer cancel()
	defer func() {
		err = errors.Join(err, queryCtx.Err())
		result.Duration = c.clock.Since(start).Seconds()
		if err != nil {
			result.Status = "error"
			result.Columns = []string{}
			result.Rows = [][]any{}
			message := err.Error()
			result.Error = &message
		} else {
			result.Status = "success"
			result.RowCount = len(result.Rows)
		}
	}()
	if err := queryCtx.Err(); err != nil {
		return result, err
	}
	if container == "" || c.cdbName == "" {
		return result, fmt.Errorf("database %q is not reachable from this Oracle integration", query.DBName)
	}
	if c.multitenant && !c.connectedToPdb {
		if _, err := c.dataObservabilityConn.ExecContext(queryCtx, dataObservabilitySetContainerSQL(container)); err != nil {
			return result, fmt.Errorf("select container %q: %w", container, err)
		}
	}
	rows, err := c.dataObservabilityConn.QueryxContext(queryCtx, query.Query)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	result.Columns, err = rows.Columns()
	if err != nil {
		return result, err
	}
	if len(result.Columns) == 0 {
		return result, errors.New("Query returned no result set — only SELECT statements are supported")
	}
	for len(result.Rows) < dataObservabilityMaxResultRows && rows.Next() {
		if err := queryCtx.Err(); err != nil {
			return result, err
		}
		values, err := rows.SliceScan()
		if err != nil {
			return result, err
		}
		result.Rows = append(result.Rows, values)
	}
	return result, rows.Err()
}

func isDataObservabilityConnectionError(err error) bool {
	var networkError net.Error
	var oracleError *network.OracleError
	return isConnectionError(err) || godror.IsBadConn(err) || errors.Is(err, sql.ErrConnDone) ||
		errors.Is(err, io.EOF) || errors.As(err, &networkError) ||
		(errors.As(err, &oracleError) && oracleError.Bad())
}
