// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

import (
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/atomicstats"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type telemetry struct {
	then    *atomic.Int64
	elapsed *atomic.Int64

	totalHits *atomic.Int64
	misses    *atomic.Int64 // this happens when we can't cope with the rate of events
	dropped   *atomic.Int64 // this happens when kafkaStatKeeper reaches capacity
}

func newTelemetry() *telemetry {
	t := &telemetry{
		then:      atomic.NewInt64(time.Now().Unix()),
		elapsed:   atomic.NewInt64(0),
		totalHits: atomic.NewInt64(0),
		misses:    atomic.NewInt64(0),
		dropped:   atomic.NewInt64(0),
	}

	return t
}

//func (t *telemetry) aggregate(transactions []*ebpfKafkaTx, err error) {
//	t.totalHits.Add(int64(len(transactions)))
//
//	if err == errLostBatch {
//		t.misses.Add(int64(len(transactions)))
//	}
//}

func (t *telemetry) log() {
	now := time.Now().Unix()
	then := t.then.Swap(now)

	delta := newTelemetry()
	delta.totalHits.Store(t.totalHits.Swap(0))
	delta.misses.Store(t.misses.Swap(0))
	delta.dropped.Store(t.dropped.Swap(0))
	delta.elapsed.Store(now - then)

	log.Debugf(
		"kafka stats summary: requests_processed=%d(%.2f/s) requests_missed=%d(%.2f/s) requests_dropped=%d(%.2f/s)",
		delta.totalHits.Load(),
		float64(delta.totalHits.Load())/float64(delta.elapsed.Load()),
		delta.misses.Load(),
		float64(delta.misses.Load())/float64(delta.elapsed.Load()),
		delta.dropped.Load(),
		float64(delta.dropped.Load())/float64(delta.elapsed.Load()),
	)
}

func (t *telemetry) report() map[string]interface{} {
	return atomicstats.Report(t)
}
