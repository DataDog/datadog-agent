// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"expvar"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	s := &Stats{
		size:       sz,
		valExpvar:  expvar.NewInt("pktsec"),
		last:       time.Now(),
		stopped:    make(chan struct{}),
		incoming:   make(chan int64, sz),
		Aggregated: make(chan Stat, 2),
	}

	return s, nil
}

// StatEvent aggregates an event with value v
func (s *Stats) StatEvent(v int64) {
	select {
	case s.incoming <- v:
		return
	default:
		log.Debugf("dropping last second stats, buffer full")
	}
}

// Process call to start processing statistics
func (s *Stats) Process() {
	t := time.NewTicker(time.Second)
	defer t.Stop()

	for {
		select {
		case v := <-s.incoming:
			s.valExpvar.Add(v)
		case <-t.C:
			select {
			case s.Aggregated <- Stat{
				Val: s.valExpvar.Value(),
				Ts:  s.last,
			}:
			default:
				log.Debugf("dropping last second stats, buffer full")
			}

			s.valExpvar.Set(0)
			s.last = time.Now()
		case <-s.stopped:
			return
		}
	}
}

// Update update the expvar parameter with the last aggregated value
func (s *Stats) Update(expStat *expvar.Int) {
	t := time.NewTicker(time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			last := <-s.Aggregated
			expStat.Set(last.Val)
		case <-s.stopped:
			return
		}
	}
}

// Stop call to stop processing statistics. Once stopped, Stats cannot be restarted.
func (s *Stats) Stop() {
	close(s.stopped)
}
