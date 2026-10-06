// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package config

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultDataObservabilityCollectionInterval = 10

type DataObservabilityConfig struct {
	Enabled            bool                     `yaml:"enabled"`
	CollectionInterval float64                  `yaml:"collection_interval"`
	RunSync            bool                     `yaml:"run_sync"`
	ConfigID           string                   `yaml:"config_id"`
	Queries            []DataObservabilityQuery `yaml:"queries"`
}

type DataObservabilityQuery struct {
	MonitorID             int64                                   `yaml:"monitor_id"`
	DBName                string                                  `yaml:"dbname"`
	Type                  string                                  `yaml:"type"`
	Query                 string                                  `yaml:"query"`
	IntervalSeconds       int64                                   `yaml:"interval_seconds"`
	Schedule              string                                  `yaml:"schedule"`
	QueryTimeout          int64                                   `yaml:"query_timeout"`
	Entity                *DataObservabilityEntity                `yaml:"entity"`
	CustomSQLSelectFields *DataObservabilityCustomSQLSelectFields `yaml:"custom_sql_select_fields"`
}

type DataObservabilityEntity struct {
	Platform string `yaml:"platform"`
	Account  string `yaml:"account"`
	Database string `yaml:"database"`
	Schema   string `yaml:"schema"`
	Table    string `yaml:"table"`
}

type DataObservabilityCustomSQLSelectFields struct {
	MetricConfigID *int64  `yaml:"metric_config_id"`
	EntityID       *string `yaml:"entity_id"`
}

func (q DataObservabilityQuery) QueryTimeoutDuration() time.Duration {
	return time.Duration(q.QueryTimeout) * time.Millisecond
}

func (c *DataObservabilityConfig) normalize() {
	if math.IsNaN(c.CollectionInterval) || c.CollectionInterval < float64(time.Nanosecond)/float64(time.Second) ||
		c.CollectionInterval > float64(math.MaxInt64/int64(time.Second)) {
		c.CollectionInterval = defaultDataObservabilityCollectionInterval
	}
	validQueries := c.Queries[:0]
	for i, q := range c.Queries {
		if err := q.validate(); err != nil {
			log.Warnf("Skipping data_observability.queries[%d] (monitor_id=%d): %v", i, q.MonitorID, err)
			continue
		}
		validQueries = append(validQueries, q)
	}
	clear(c.Queries[len(validQueries):])
	c.Queries = validQueries
}

func (q DataObservabilityQuery) validate() error {
	if strings.TrimSpace(q.DBName) == "" {
		return errors.New("dbname is required")
	}
	if strings.TrimSpace(q.Query) == "" {
		return errors.New("query is required")
	}
	if q.QueryTimeout <= 0 || q.QueryTimeout > math.MaxInt64/int64(time.Millisecond) {
		return errors.New("query_timeout must be a positive duration in milliseconds")
	}
	if q.Entity == nil {
		return errors.New("entity is required")
	}
	if q.Schedule != "" {
		if len(strings.Fields(q.Schedule)) != 5 {
			return errors.New("schedule must be a five-field cron expression")
		}
		if _, err := cron.ParseStandard(q.Schedule); err != nil {
			return fmt.Errorf("invalid schedule: %w", err)
		}
	} else if q.IntervalSeconds <= 0 || q.IntervalSeconds > math.MaxInt64/int64(time.Second) {
		return errors.New("interval_seconds must be a positive duration in seconds when schedule is unset")
	}
	return nil
}
