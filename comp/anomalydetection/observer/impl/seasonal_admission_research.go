// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetectiontestbench

package observerimpl

import (
	"container/list"
	"os"
	"sort"
	"strconv"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// Research scheduling policy, independent of candidate eligibility and model fit.
// Entries and waiting nodes are removed with storage refs; their bound is the
// upstream storage series cap (1,200 in this experiment). Expensive learning
// state has an additional, smaller cap, including the calibration stage.
type seasonalAdmissionEntry struct {
	readStamp    seasonalReadStamp
	discovered   int64
	latest       int64
	start        int64
	active, done bool
	waiting      *list.Element
}

type seasonalAdmissionBudget struct {
	limit, active, peak                                   int
	queue                                                 list.List
	entries                                               map[observer.SeriesRef]*seasonalAdmissionEntry
	grants, completions, failures, removals, idleReleases uint64
}

const seasonalAdmissionIdleSeconds int64 = 60

func newSeasonalAdmissionBudget(limit int) *seasonalAdmissionBudget {
	if limit < 1 || limit > 1200 {
		panic("seasonal learner limit must be 1..1200")
	}
	return &seasonalAdmissionBudget{limit: limit, entries: make(map[observer.SeriesRef]*seasonalAdmissionEntry)}
}

func seasonalAdmissionFromEnvironment() *seasonalAdmissionBudget {
	v := os.Getenv("OBSERVER_REAL_SEASONAL_LEARNERS")
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(err)
	}
	return newSeasonalAdmissionBudget(n)
}

func (b *seasonalAdmissionBudget) touch(ref observer.SeriesRef, latest int64) {
	e := b.entries[ref]
	if e == nil {
		e = &seasonalAdmissionEntry{}
		b.entries[ref] = e
	}
	if latest <= e.latest {
		return
	}
	e.latest = latest
}

func (b *seasonalAdmissionBudget) finish(ref observer.SeriesRef, success bool) {
	e := b.entries[ref]
	if e == nil || e.done {
		return
	}
	if e.active {
		b.active--
		e.active = false
	}
	if e.waiting != nil {
		b.queue.Remove(e.waiting)
		e.waiting = nil
	}
	e.done = true
	if success {
		b.completions++
	} else {
		b.failures++
	}
}

func (b *seasonalAdmissionBudget) remove(ref observer.SeriesRef) {
	e := b.entries[ref]
	if e == nil {
		return
	}
	if e.active {
		b.active--
	}
	if e.waiting != nil {
		b.queue.Remove(e.waiting)
	}
	delete(b.entries, ref)
	b.removals++
}

func (d *realSeasonalDetector) prepareAdmission(storage observer.StorageReader, metas []observer.SeriesMeta, now int64) {
	b := d.budget
	if b == nil {
		return
	}
	// Numeric storage handles break simultaneous-arrival ties. Neither metric
	// names nor values nor fault labels influence queue order. Sort only refs
	// newly observed, keeping existing waiters ahead of later arrivals.
	newMetas := make([]observer.SeriesMeta, 0)
	for _, meta := range metas {
		if !realSeasonalEligible(meta) {
			continue
		}
		if d.states[meta.Ref] == nil {
			newMetas = append(newMetas, meta)
		}
	}
	sort.Slice(newMetas, func(i, j int) bool { return newMetas[i].Ref < newMetas[j].Ref })
	for _, meta := range newMetas {
		d.states[meta.Ref] = &realSeasonalGaugeState{detector: realSeasonalGaugeDetector(d.factory, observer.AggregateAverage, 0), countDetector: realSeasonalGaugeDetector(d.factory, observer.AggregateCount, 0)}
		b.entries[meta.Ref] = &seasonalAdmissionEntry{discovered: now}
	}
	arrivals := make([]observer.SeriesRef, 0)
	for _, meta := range metas {
		e := b.entries[meta.Ref]
		if e == nil || e.done {
			continue
		}
		if e.readStamp.changed(storage, meta.Ref, now) {
			storage.ForEachPoint(meta.Ref, e.latest, now, observer.AggregateAverage, func(_ *observer.Series, p observer.Point) { b.touch(meta.Ref, p.Timestamp) })
		}
		if e.active && (now-e.latest > seasonalAdmissionIdleSeconds || now-e.start > int64(realSeasonalTrain+realSeasonalCalibration)*realSeasonalStep+seasonalAdmissionIdleSeconds) {
			b.idleReleases++
			d.disqualify(meta, d.states[meta.Ref], "learning_lease_expired")
		}
		if !e.active && !e.done && e.waiting == nil && e.latest > 0 && now-e.latest <= seasonalAdmissionIdleSeconds {
			arrivals = append(arrivals, meta.Ref)
		}
	}
	// Discovery can precede the first visible point. Order every same-advance
	// arrival, not only newly created storage identities.
	sort.Slice(arrivals, func(i, j int) bool { return arrivals[i] < arrivals[j] })
	for _, ref := range arrivals {
		b.entries[ref].waiting = b.queue.PushBack(ref)
	}
	for b.active < b.limit && b.queue.Len() > 0 {
		node := b.queue.Front()
		ref := node.Value.(observer.SeriesRef)
		e := b.entries[ref]
		b.queue.Remove(node)
		e.waiting = nil
		if now-e.latest > seasonalAdmissionIdleSeconds {
			continue
		}
		e.active = true
		e.start = now
		b.active++
		b.peak = max(b.peak, b.active)
		b.grants++
		s := d.states[ref]
		model, err := newHourlyAdaptiveSeasonalModel(15, []int{1800, 3600})
		if err != nil {
			panic(err)
		}
		s.model = model
		// Start with the latest observed point; never reconstruct waiting history
		// or reset the raw detector. Subsequent learning is strictly causal.
		s.last = e.latest - 1
		if e.discovered == now {
			s.last = 0
		} // Immediate admission preserves the original first snapshot.
	}
}

func (b *seasonalAdmissionBudget) census() map[string]any {
	return map[string]any{"limit": b.limit, "active": b.active, "peak": b.peak, "waiting": b.queue.Len(), "entries": len(b.entries), "grants": b.grants, "completions": b.completions, "failures": b.failures, "removals": b.removals, "idle_releases": b.idleReleases}
}
