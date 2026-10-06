// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"time"

	"github.com/robfig/cron/v3"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const dataObservabilityCronStartupLookback = 300 * time.Second

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
