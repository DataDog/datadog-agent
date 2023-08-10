// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//revive:disable:var-naming

//go:build oracle

package oracle

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	godror "github.com/godror/godror"
	"reflect"
	"strconv"
)

// Method exported type should have comment or be unexported
type Method func(string, float64, string, []string)

type metricRow struct {
	name   string
	value  float64
	method Method
	tags   []string
}

func logTypeError(prefix string, expectedType string, column string, value interface{}, query string) {
	log.Errorf(
		`Custom query %s encountered a type error during execution. A %s was expected for the column %s, but the query results returned the value "%v" of type %s. Query was: "%s"`,
		prefix, expectedType, column, value, reflect.TypeOf(value), query,
	)
}

// CustomQueries exported method should have comment or be unexported
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
			if errClosing := CloseDatabaseConnection(db); err != nil {
				log.Errorf("Error closing connection %s", errClosing)
			}
			return err
		}
		if db == nil {
			return fmt.Errorf("empty connection")
		}
		c.dbCustomQueries = db
		c.dbCustomQueries.SetMaxOpenConns(1)
	}

	var metricRows []metricRow
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to get sender for custom queries %w", err)
	}
	methods := map[string]Method{
		"gauge":           sender.Gauge,
		"count":           sender.Count,
		"rate":            sender.Rate,
		"monotonic_count": sender.MonotonicCount,
		"histogram":       sender.Histogram,
		"historate":       sender.Historate,
	}
	for _, q := range c.config.CustomQueries {
		var errInQuery bool
		metricPrefix := q.MetricPrefix

		if metricPrefix == "" {
			log.Error("Undefined metric_prefix for a custom query")
			continue
		}
		log.Tracef("custom query configuration %v", q)
		var pdb string
		if !c.connectedToPdb {
			pdb = q.Pdb
			if pdb == "" {
				pdb = "cdb$root"
			}
			_, err := c.dbCustomQueries.Exec(fmt.Sprintf("alter session set container = %s", pdb))
			if err != nil {
				log.Errorf("Can't set container to %s", pdb)
				continue
			}
		}
		rows, err := c.dbCustomQueries.Queryx(q.Query)
		if rows != nil {
			defer rows.Close()
		}
		if err != nil {
			log.Errorf("failed to fetch rows for the custom query %s %s", metricPrefix, err)
			continue
		}
		for rows.Next() {
			var metricsFromSingleRow []metricRow
			tags := []string{fmt.Sprintf("pdb:%s", pdb)}
			cols, err := rows.SliceScan()
			if err != nil {
				log.Errorf("failed to get values for the custom query %s %s", metricPrefix, err)
				errInQuery = true
				break
			}
			if len(cols) > len(q.Columns) {
				log.Errorf("Not enough column mappings for the custom query %s %s", metricPrefix, err)
				errInQuery = true
				break
			}
			var metricRow metricRow
			for i, v := range cols {
				if q.Columns[i].Type == "tag" {
					if v != nil {
						tags = append(tags, fmt.Sprintf("%s:%s", q.Columns[i].Name, v))
					}
				} else if methodFunc, ok := methods[q.Columns[i].Type]; ok {
					metricRow.name = fmt.Sprintf("%s.%s", metricPrefix, q.Columns[i].Name)
					// don't use underscores in Go names; var v_str should be vStr
					if v_str, ok := v.(string); ok {
						metricRow.value, err = strconv.ParseFloat(v_str, 64)
						if err != nil {
							logTypeError(metricPrefix, "number", metricRow.name, v, q.Query)
							errInQuery = true
							break
						}
						// don't use underscores in Go names; var v_gn should be vGn
					} else if v_gn, ok := v.(godror.Number); ok {
						metricRow.value, err = strconv.ParseFloat(string(v_gn), 64)
						if err != nil {
							logTypeError(metricPrefix, "godror.Number", metricRow.name, v, q.Query)
							errInQuery = true
							break
						}
					} else {
						logTypeError(metricPrefix, "godror.Number", metricRow.name, v, q.Query)
						errInQuery = true
						break
					}

					metricRow.method = methodFunc
					metricsFromSingleRow = append(metricsFromSingleRow, metricRow)
				} else {
					log.Errorf("Unknown column type %s in custom query %s", q.Columns[i].Type, metricRow.name)
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
			for i := range metricsFromSingleRow {
				metricsFromSingleRow[i].tags = tags
			}
			metricRows = append(metricRows, metricsFromSingleRow...)

		}
		rows.Close()
		if errInQuery {
			continue
		}

		for _, m := range metricRows {
			log.Tracef("send metric %+v", m)
			m.method(m.name, m.value, "", m.tags)
		}
		sender.Commit()
	}
	return nil
}
