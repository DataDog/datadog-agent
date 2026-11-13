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
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	godror "github.com/godror/godror"
)

type metricRow struct {
	name   string
	value  float64
	method metricType
	tags   []string
}

func concatenateTypeError(input error, prefix string, expectedType string, column string, value interface{}, query string, err error) error {
	newErr := fmt.Errorf(
		`Custom query %s encountered a type error during execution. A %s was expected for the column %s, but the query results returned the value "%v" of type %s. Query was: "%s". Error: %w`,
		prefix, expectedType, column, value, reflect.TypeOf(value), query, err,
	)
	return errors.Join(input, newErr)
}

func concatenateError(input error, new string) error {
	return errors.Join(input, errors.New(new))
}

func shouldExecuteCustomQuery(lastExecutionTime *time.Time, collectionInterval *int64, now time.Time) bool {
	if collectionInterval == nil {
		return true
	}
	if lastExecutionTime.IsZero() || now.Sub(*lastExecutionTime).Seconds() >= float64(*collectionInterval) {
		*lastExecutionTime = now
		return true
	}
	return false
}

//nolint:revive // TODO(DBM) Fix revive linter
func (c *Check) CustomQueries() error {
	var customQueries []config.CustomQuery

	if len(c.config.InstanceConfig.CustomQueries) > 0 {
		customQueries = append(customQueries, c.config.InstanceConfig.CustomQueries...)
	}
	if len(c.config.InitConfig.CustomQueries) > 0 {
		switch c.config.UseGlobalCustomQueries {
		case "true":
			customQueries = make([]config.CustomQuery, len(c.config.InitConfig.CustomQueries))
			copy(customQueries, c.config.InitConfig.CustomQueries)
		case "false":
		case "extend":
			customQueries = append(customQueries, c.config.InitConfig.CustomQueries...)
		default:
			return fmt.Errorf(`Wrong value "%s" for the config parameter use_global_custom_queries. Valid values are "true", "false" and "extend"`, c.config.UseGlobalCustomQueries)
		}
	}

	if len(c.customQueryLastRuns) != len(customQueries) {
		c.customQueryLastRuns = make([]time.Time, len(customQueries))
	}

	var allErrors error
	for i, q := range customQueries {
		// Evaluate each query immediately before starting it. Using one timestamp for the
		// entire batch would let an earlier slow query shorten the effective interval of
		// every query that follows it.
		if !shouldExecuteCustomQuery(&c.customQueryLastRuns[i], q.CollectionInterval, c.clock.Now()) {
			continue
		}

		metricRows, err := c.runCustomQuery(q)
		if err != nil {
			allErrors = errors.Join(allErrors, err)
			continue
		}
		for _, m := range metricRows {
			log.Debugf("%s send metric %+v", c.logPrompt, m)
			sendMetric(c, m.method, m.name, m.value, m.tags)
		}
		commit(c)
	}
	return allErrors
}

// runCustomQuery releases rows before recovering its pool, and keeps the query
// context alive until fetching and cleanup both finish. Each subsequent query
// (and recovery connection) receives an independent deadline.
func (c *Check) runCustomQuery(q config.CustomQuery) (_ []metricRow, retErr error) {
	/*
	 * We are creating a dedicated DB connection for custom queries. Custom queries is
	 * the only feature that switches to PDBs (all other queries are running against the
	 * root container). Switching to PDB and subsequent query execution isn't atomic, so
	 * there's no guarantee the both operations would get the same connection from the pool.
	 * The connection is created lazily so skipped queries do not open one unnecessarily.
	 */
	if c.dbCustomQueries == nil {
		db, err := c.Connect()
		if err != nil {
			closeDatabase(c, db)
			return nil, err
		}
		if db == nil {
			return nil, errors.New("empty connection")
		}
		c.dbCustomQueries = db
	}

	var metricRows []metricRow
	var allErrors error
	var errInQuery bool
	metricPrefix := q.MetricPrefix

	if metricPrefix == "" {
		metricPrefix = "oracle"
	}
	log.Debugf("%s custom query configuration %v", c.logPrompt, q)
	var pdb string
	if !c.connectedToPdb {
		pdb = q.Pdb
		if pdb == "" {
			pdb = "CDB$ROOT"
		}
		err := execWithDeadline(c, c.dbCustomQueries, fmt.Sprintf(`alter session set container = "%s"`, strings.ReplaceAll(pdb, `"`, `""`)))
		if err != nil {
			return nil, fmt.Errorf("failed to set container %s: %w", pdb, handleError(c, &c.dbCustomQueries, err))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.config.QueryTimeoutDuration())
	defer cancel()
	// Cleanup and deadline classification must precede recovery and cancellation.
	defer func() { retErr = handleError(c, &c.dbCustomQueries, operationError(ctx, retErr)) }()
	rows, err := c.dbCustomQueries.QueryxContext(ctx, q.Query)
	if rows != nil {
		defer func() { retErr = errors.Join(retErr, rows.Err(), rows.Close()) }()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch rows for the custom query %s: %w", metricPrefix, err)
	}
	for rows.Next() {
		var metricsFromSingleRow []metricRow
		var tags []string
		if pdb != "" {
			tags = []string{"pdb:" + pdb}
		}
		cols, err := rows.SliceScan()
		if err != nil {
			return nil, fmt.Errorf("failed to get values for the custom query %s: %w", metricPrefix, err)
		}
		if len(cols) != len(q.Columns) {
			return nil, fmt.Errorf(
				"column count mismatch for custom query %s: query returned %d columns but %d mappings configured",
				metricPrefix, len(cols), len(q.Columns))
		}
		var metricRow metricRow
		for i, v := range cols {
			if q.Columns[i].Type == "tag" {
				if v != nil {
					tags = append(tags, fmt.Sprintf("%s:%s", q.Columns[i].Name, v))
				}
			} else if method, err := getMetricFunctionCode(q.Columns[i].Type); err == nil {
				metricRow.name = fmt.Sprintf("%s.%s", metricPrefix, q.Columns[i].Name)
				if v == nil {
					allErrors = concatenateError(allErrors, fmt.Sprintf(
						"NULL value for metric column %s in custom query %s, skipping row",
						q.Columns[i].Name, metricPrefix))
					errInQuery = true
					break
				}
				if vStr, ok := v.(string); ok {
					metricRow.value, err = strconv.ParseFloat(vStr, 64)
					if err != nil {
						allErrors = concatenateTypeError(allErrors, metricPrefix, "number", metricRow.name, v, q.Query, err)
						errInQuery = true
						break
					}
				} else if v_gn, ok := v.(godror.Number); ok { //nolint:revive // TODO(DBM) Fix revive linter
					metricRow.value, err = strconv.ParseFloat(string(v_gn), 64)
					if err != nil {
						allErrors = concatenateTypeError(allErrors, metricPrefix, "godror.Number", metricRow.name, v, q.Query, err)
						errInQuery = true
						break
					}
				} else if vInt64, ok := v.(int64); ok {
					metricRow.value = float64(vInt64)
				} else if vFloat64, ok := v.(float64); ok {
					metricRow.value = vFloat64
				} else {
					allErrors = concatenateTypeError(allErrors, metricPrefix, "UNKNOWN", metricRow.name, v, q.Query, err)
					errInQuery = true
					break
				}

				metricRow.method = method
				metricsFromSingleRow = append(metricsFromSingleRow, metricRow)
			} else {
				allErrors = concatenateError(allErrors, fmt.Sprintf("Unknown column type %s in custom query %s", q.Columns[i].Type, metricRow.name))
				errInQuery = true
				break
			}
		}
		if errInQuery {
			break
		}
		tags = append(c.tags, tags...)
		if len(q.Tags) > 0 {
			tags = append(tags, q.Tags...)
		}
		log.Debugf("%s Appended queried tags to check tags %v", c.logPrompt, tags)
		for i := range metricsFromSingleRow {
			metricsFromSingleRow[i].tags = make([]string, len(tags))
			copy(metricsFromSingleRow[i].tags, tags)
		}
		metricRows = append(metricRows, metricsFromSingleRow...)
	}
	return metricRows, allErrors
}
