package util

import (
	"sync/atomic"
	"time"

	log "github.com/cihub/seelog"
)

type Stat struct {
	Val int64
	Ts  time.Time
}

type StatOperator func(int64, int64) int64

type Stats struct {
	size       uint32
	val        int64
	operator   StatOperator
	running    uint32
	last       time.Time
	incoming   chan int64
	Aggregated chan Stat
}

func NewStats(op StatOperator, sz uint32) (*Stats, error) {
	s := &Stats{
		size:       sz,
		val:        0,
		operator:   op,
		running:    0,
		last:       time.Now(),
		incoming:   make(chan int64, sz),
		Aggregated: make(chan Stat, 2),
	}

	return s, nil
}

func (s *Stats) StatEvent(v int64) {
	select {
	case s.incoming <- v:
		return
	default:
		log.Debugf("dropping last second stasts, buffer full")
	}
}

func (s *Stats) Process() {
	tickChan := time.NewTicker(time.Second).C
	atomic.StoreUint32(&s.running, 1)
	for {
		select {
		case v := <-s.incoming:
			s.val = s.operator(s.val, v)
		case <-tickChan:
			select {
			case s.Aggregated <- Stat{
				Val: s.val,
				Ts:  s.last,
			}:
			default:
				log.Debugf("dropping last second stasts, buffer full")
			}
			s.val = 0
			s.last = time.Now()
			if atomic.LoadUint32(&s.running) == 0 {
				break
			}
		}
	}
}

func (s *Stats) Stop() {
	atomic.StoreUint32(&s.running, 0)
}
