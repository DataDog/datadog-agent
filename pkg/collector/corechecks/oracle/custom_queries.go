// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
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
	return fmt.Errorf("%w %s", input, new)
}

//nolint:revive // TODO(DBM) Fix revive linter
func (c *Check) CustomQueries() error {
	/*
	 * We are creating a dedicated DB connection for custom queries. Custom queries is
	 * the only feature that switches to PDBs (all other queries are running against the
	 * root container). Switching to PDB and subsequent query execution isn't atomic, so
	 * there's no guarantee the both operations would get the same connection from the pool.
	 */
	if c.dbCustomQueries == nil {
		db, err := c.Connect()
		if err != nil {
			closeDatabase(c, db)
			return err
		}
		if db == nil {
			return errors.New("empty connection")
		}
		c.dbCustomQueries = db
	}

	var metricRows []metricRow
	var allErrors error
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

	for _, q := range customQueries {
		metricRows = metricRows[:0]
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
				pdb = "cdb$root"
			}
			_, err := c.dbCustomQueries.Exec(fmt.Sprintf(`alter session set container = "%s"`, strings.ReplaceAll(pdb, `"`, `""`)))
			if err != nil {
				allErrors = concatenateError(allErrors, fmt.Sprintf("failed to set container %s %s", pdb, err))
				reconnectOnConnectionError(c, &c.dbCustomQueries, err)
				continue
			}
		}
		var queryStartTimestamp float64
		if q.MetricTimestamp == "query_start" {
			queryStartTimestamp = float64(time.Now().UnixNano()) / float64(time.Second)
		}
		rows, err := c.dbCustomQueries.Queryx(q.Query)
		if rows != nil {
			defer rows.Close()
		}
		if err != nil {
			allErrors = concatenateError(allErrors, fmt.Sprintf("failed to fetch rows for the custom query %s %s", metricPrefix, err))
			continue
		}
		for rows.Next() {
			var metricsFromSingleRow []metricRow
			var tags []string
			if pdb != "" {
				tags = []string{"pdb:" + pdb}
			}
			cols, err := rows.SliceScan()
			if err != nil {
				allErrors = concatenateError(allErrors, fmt.Sprintf("failed to get values for the custom query %s %s", metricPrefix, err))
				errInQuery = true
				break
			}
			if len(cols) != len(q.Columns) {
				allErrors = concatenateError(allErrors, fmt.Sprintf(
					"column count mismatch for custom query %s: query returned %d columns but %d mappings configured",
					metricPrefix, len(cols), len(q.Columns)))
				errInQuery = true
				break
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
					if v_str, ok := v.(string); ok {
						metricRow.value, err = strconv.ParseFloat(v_str, 64)
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
		rows.Close()
		if errInQuery {
			continue
		}
		for _, m := range metricRows {
			log.Debugf("%s send metric %+v", c.logPrompt, m)
			if q.MetricTimestamp == "query_start" {
				sendMetricWithTimestamp(c, m.method, m.name, m.value, m.tags, queryStartTimestamp)
			} else {
				sendMetric(c, m.method, m.name, m.value, m.tags)
			}
		}
		commit(c)
	}
	return allErrors
}
