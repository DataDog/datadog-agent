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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/godror/godror"
	"github.com/jmoiron/sqlx"
	"github.com/robfig/cron/v3"
	"github.com/sijms/go-ora/v2/network"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
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

	startedAt   time.Time
	columnTypes []*sql.ColumnType
}

type dataObservabilityEvent struct {
	Timestamp             int64                                          `json:"timestamp"`
	ConfigID              string                                         `json:"config_id"`
	DBType                string                                         `json:"db_type"`
	DBHost                string                                         `json:"db_host"`
	DBPort                int                                            `json:"db_port"`
	DBName                string                                         `json:"db_name"`
	MonitorID             int64                                          `json:"monitor_id"`
	Query                 string                                         `json:"query"`
	Entity                any                                            `json:"entity"`
	CustomSQLSelectFields *config.DataObservabilityCustomSQLSelectFields `json:"custom_sql_select_fields"`
	dataObservabilityQueryResult
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
	c.dataObservabilityWorkerMu.Lock()
	emit := c.dataObservabilityEmitter
	c.dataObservabilityWorkerMu.Unlock()
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
		if emit != nil {
			emit(due, result)
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
	result.startedAt = start
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
	var options []any
	if c.dataObservabilityDB.DriverName() == common.Godror {
		// String fetching makes NULL and empty CLOBs indistinguishable.
		options = append(options, godror.LobAsReader())
	}
	rows, err := c.dataObservabilityConn.QueryxContext(queryCtx, query.Query, options...)
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
	result.columnTypes, err = rows.ColumnTypes()
	if err != nil {
		return result, err
	}
	for len(result.Rows) < dataObservabilityMaxResultRows && rows.Next() {
		if err := queryCtx.Err(); err != nil {
			return result, err
		}
		values, err := rows.SliceScan()
		if err != nil {
			return result, err
		}
		for i, value := range values {
			if lob, ok := value.(*godror.Lob); ok {
				values[i], err = readDataObservabilityLob(lob)
				if err != nil {
					return result, err
				}
			}
		}
		result.Rows = append(result.Rows, values)
	}
	return result, rows.Err()
}

func readDataObservabilityLob(lob *godror.Lob) (value any, err error) {
	if lob == nil || lob.Reader == nil {
		return nil, nil
	}
	if closer, ok := lob.Reader.(io.Closer); ok {
		defer func() { err = errors.Join(err, closer.Close()) }()
	}
	data, err := io.ReadAll(lob)
	if err != nil {
		return nil, err
	}
	if lob.IsClob {
		return string(data), nil
	}
	return data, nil
}

func isDataObservabilityConnectionError(err error) bool {
	var networkError net.Error
	var oracleError *network.OracleError
	return isConnectionError(err) || godror.IsBadConn(err) || errors.Is(err, sql.ErrConnDone) ||
		errors.Is(err, io.EOF) || errors.As(err, &networkError) ||
		(errors.As(err, &oracleError) && oracleError.Bad())
}

func (c *Check) newDataObservabilityEmitter() (func(dataObservabilityDueQuery, dataObservabilityQueryResult), error) {
	sender, err := c.GetSender()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize data observability sender: %w", err)
	}
	var baseTags []string
	for _, tag := range c.tags {
		if !strings.HasPrefix(tag, "dd.internal") {
			baseTags = append(baseTags, tag)
		}
	}
	configID := c.config.DataObservability.ConfigID
	if configID != "" {
		baseTags = append(baseTags, "config_id:"+configID)
	}
	baseTags = append(baseTags, "db_type:oracle")
	hostname, port, runClock := c.dbHostname, c.config.Port, c.clock
	return func(due dataObservabilityDueQuery, result dataObservabilityQueryResult) {
		query := due.scheduledQuery.query
		tags := append(slices.Clone(baseTags), fmt.Sprintf("monitor_id:%d", query.MonitorID))
		sender.Gauge("dd.oracle.data_observability.query_execution_time", result.Duration, hostname, tags)
		sender.Count("dd.oracle.data_observability.query_executions", 1, hostname, append(slices.Clone(tags), "status:"+result.Status))
		mode := "interval"
		if due.scheduledQuery.schedule != nil {
			mode = "cron"
		}
		lateness := max(0, result.startedAt.Sub(due.scheduledTime).Seconds())
		sender.Gauge("dd.oracle.data_observability.query_fire_lateness_seconds", lateness, hostname, append(slices.Clone(tags), "mode:"+mode))

		var entity any = map[string]any{}
		if query.Entity != nil {
			entity = query.Entity
		}
		event := dataObservabilityEvent{
			Timestamp: runClock.Now().UnixMilli(), ConfigID: configID,
			DBType: "oracle", DBHost: hostname, DBPort: port,
			DBName: query.DBName, MonitorID: query.MonitorID, Query: query.Query,
			Entity: entity, CustomSQLSelectFields: query.CustomSQLSelectFields,
			dataObservabilityQueryResult: result,
		}
		payload, err := event.marshal()
		if err != nil {
			log.Warnf("Failed to emit metrics/event for monitor_id=%d: %v", query.MonitorID, err)
			excClass := reflect.TypeOf(err)
			if excClass.Kind() == reflect.Pointer {
				excClass = excClass.Elem()
			}
			sender.Count("dd.oracle.data_observability.emit_failures", 1, hostname, append(tags, "exc_class:"+excClass.Name()))
			return
		}
		log.Debugf("Query result for monitor_id=%d: status=%s row_count=%d", query.MonitorID, result.Status, result.RowCount)
		sender.EventPlatformEvent(payload, "do-query-results")
	}, nil
}

func (e dataObservabilityEvent) marshal() ([]byte, error) {
	rows := make([][]any, len(e.Rows))
	for i, row := range e.Rows {
		rows[i] = make([]any, len(row))
		for j, value := range row {
			var databaseType string
			if j < len(e.columnTypes) {
				databaseType = e.columnTypes[j].DatabaseTypeName()
			}
			normalized, err := normalizeDataObservabilityValue(value, databaseType)
			if err != nil {
				return nil, err
			}
			rows[i][j] = normalized
		}
	}
	e.Rows = rows
	return json.Marshal(e)
}

func normalizeDataObservabilityValue(value any, databaseType string) (any, error) {
	switch v := value.(type) {
	case nil, bool, int64, uint64:
		return v, nil
	case string:
		if databaseType == "NUMBER" {
			return strconv.ParseFloat(v, 64)
		}
		if v == "" {
			switch databaseType {
			case "VARCHAR2", "NVARCHAR2", "CHAR", "NCHAR", "LONG", "XMLTYPE":
				return nil, nil
			}
		}
		return v, nil
	case godror.Number:
		return strconv.ParseFloat(string(v), 64)
	case float64:
		// The Python integrations use orjson, which encodes non-finite floats as null.
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, nil
		}
		return v, nil
	case float32:
		return normalizeDataObservabilityValue(float64(v), databaseType)
	case []byte:
		if !utf8.Valid(v) {
			return nil, &json.UnsupportedValueError{Value: reflect.ValueOf(v), Str: "invalid UTF-8"}
		}
		return string(v), nil
	case time.Time:
		layout := "2006-01-02T15:04:05"
		if v.Nanosecond()/1000 != 0 {
			layout += ".000000"
		}
		switch databaseType {
		case "TIMESTAMP WITH TIME ZONE", "TIMESTAMP WITH LOCAL TIME ZONE", "TimeStampTZ_DTY", "TimeStampTZ", "TimeStampLTZ_DTY", "TimeStampeLTZ":
			layout += "-07:00"
		}
		return v.Format(layout), nil
	default:
		return nil, &json.UnsupportedTypeError{Type: reflect.TypeOf(value)}
	}
}
