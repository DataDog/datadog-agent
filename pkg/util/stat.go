// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"expvar"
	"time"
)

// Stat type includes a statted value and its timestamp.
type Stat struct {
	Val int64
	Ts  time.Time
}

// Stats type structure enabling statting facilities.
type Stats struct {
	size       uint32
	valExpvar  *expvar.Int
	last       time.Time
	stopped    chan struct{}
	incoming   chan int64
	Aggregated chan Stat
}

// NewStats constructor for Stats
func NewStats(sz uint32) (*Stats, error) {
	panic("not called")
}

// StatEvent aggregates an event with value v
func (s *Stats) StatEvent(v int64) {
	panic("not called")
}

// Process call to start processing statistics
func (s *Stats) Process() {
	panic("not called")
}

// Update update the expvar parameter with the last aggregated value
func (s *Stats) Update(expStat *expvar.Int) {
	panic("not called")
}

// Stop call to stop processing statistics. Once stopped, Stats cannot be restarted.
func (s *Stats) Stop() {
	panic("not called")
}
