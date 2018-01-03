// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package util

import (
	"expvar"
	"strconv"
	"sync/atomic"
	"time"

	log "github.com/cihub/seelog"
)

// Stat type includes a statted value and its timestamp.
type Stat struct {
	Val int64
	Ts  time.Time
}

// Stats type structure enabling statting facilities.
type Stats struct {
	size       uint32
	running    uint32
	valExpvar  *expvar.Int
	last       time.Time
	incoming   chan int64
	Aggregated chan Stat
}

// NewStats constructor for Stats
func NewStats(sz uint32) (*Stats, error) {
	s := &Stats{
		size:       sz,
		running:    0,
		valExpvar:  expvar.NewInt("pktsec"),
		last:       time.Now(),
		incoming:   make(chan int64, sz),
		Aggregated: make(chan Stat, 3),
	}

	return s, nil
}

// StatEvent aggregates an event with value v
func (s *Stats) StatEvent(v int64) {
	select {
	case s.incoming <- v:
		return
	default:
		log.Debugf("dropping last second stasts, buffer full")
	}
}

// Process call to start processing statistics
func (s *Stats) Process() {
	tickChan := time.NewTicker(time.Second).C
	atomic.StoreUint32(&s.running, 1)
	for {
		select {
		case v := <-s.incoming:
			s.valExpvar.Add(v)
		case <-tickChan:
			// once we're fully on 1.8 get rid of this nonesense and use Value()
			pkts, err := strconv.ParseInt(s.valExpvar.String(), 10, 64)
			if err != nil {
				log.Debugf("error converting metric: %s", err)
				continue
			}

			select {
			case s.Aggregated <- Stat{
				Val: pkts,
				Ts:  s.last,
			}:
			default:
				log.Debugf("dropping last second stasts, buffer full")
			}
			s.valExpvar.Set(0)
			s.last = time.Now()
			if atomic.LoadUint32(&s.running) == 0 {
				break
			}
		}
	}
}

// Stop call to stop processing statistics
func (s *Stats) Stop() {
	atomic.StoreUint32(&s.running, 0)
}
