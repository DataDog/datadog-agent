// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strconv"
)

type Method func(string, float64, string, []string)

type metricRow struct {
	name   string
	value  float64
	method Method
	tags   []string
}

func (c *Check) CustomQueries() error {
	var metricRows []metricRow
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to get sender for custom queries %w", err)
	}
	methods := map[string]Method{"gauge": sender.Gauge, "count": sender.Count, "rate": sender.Rate}
	for _, q := range c.config.CustomQueries {
		var errInQuery bool
		metricPrefix := q.MetricPrefix
		log.Tracef("custom query structure %+v", q)
		if metricPrefix == "" {
			log.Error("Undefined metric_prefix for a custom query")
			continue
		}
		rows, err := c.db.Queryx(q.Query)
		if rows != nil {
			defer rows.Close()
		}
		if err != nil {
			log.Errorf("failed to fetch rows for the custom query %s %s", metricPrefix, err)
			continue
		}
		for rows.Next() {
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
			var tags []string
			var metricRow metricRow
			for i, v := range cols {
				if q.Columns[i].Type == "tag" {
					tags = append(tags, fmt.Sprintf("%s:%s", q.Columns[i].Name, v))
				} else if methodFunc, ok := methods[q.Columns[i].Type]; ok {
					metricRow.name = fmt.Sprintf("%s.%s", metricPrefix, q.Columns[i].Name)
					if v_str, ok := v.(string); ok {
						metricRow.value, err = strconv.ParseFloat(v_str, 64)
						if err != nil {
							log.Errorf("Metric value %v for metricRow.name %s is not a number", v, metricRow.name)
							errInQuery = true
							break
						}
					} else {
						log.Errorf("Can't parse metric value %v for metricRow.name %sr", v, metricRow.name)
						errInQuery = true
						break
					}

					metricRow.method = methodFunc
				} else {
					log.Errorf("Unknown column type %s in custom query %s", q.Columns[i].Type, metricRow.name)
					errInQuery = true
					break
				}
			}
			if errInQuery {
				continue
			}
			tags = append(c.tags, tags...)
			if len(q.Tags) > 0 {
				tags = append(tags, q.Tags...)
			}
			metricRow.tags = tags
			metricRows = append(metricRows, metricRow)
		}
	}
	log.Tracef("Custom queries %v", metricRows)
	return nil
}
