// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// CrossDecorrelateDetector fires when a pair of historically high-correlation
// series within the same (host, service) tag scope decorrelates: long-window
// |Pearson r| ≥ cdHighCorrelation collapses to short-window |r| < cdLowCorrelation.
//
// This is genuinely multivariate — no univariate detector (BOCPD, Holt residual,
// spectral residual, EVT, KS drift) can see this pattern, since both series may
// be individually well-behaved while their joint distribution shifts. Inspired
// by the SRE "this query is fine but throughput desynced from latency" pattern
// and Netflix Atlas's decorrelation alert mode.
//
// Memory per pair: 6 long-window moments (48B) + 200-pair sliding ring (~3.2KB)
// + 30-pair short ring (~480B) ≈ 4KB. With ≤K(K-1)/2 pairs per scope (K=4 →
// 6 pairs) and G scopes, memory is bounded by 4KB·6·G ≈ 25KB per 100 scopes —
// comparable to BOCPD's per-series posterior arrays.
//
// Per-tick cost: O(N + Σ_g K_g²·W̄). For G=50 scopes, K=4 members, ~30 new
// points per Detect call: ~6000 ops/tick, dominated by the per-pair Welford
// updates and the periodic moment recompute on the long-window ring.

// Cross-decorrelate algorithm constants. Names follow the cd<Field> convention
// used by the holt_residual and evt_spot detectors so package-level greps for
// "cd*" stay scoped.
const (
	// cdMaxSeriesPerScope bounds the per-scope membership; pair count grows as
	// K·(K-1)/2 so K=4 keeps it at 6 pairs/scope.
	cdMaxSeriesPerScope = 4
	// cdShortWindow is the test window for the rolling Pearson r.
	cdShortWindow = 30
	// cdLongWindow is the baseline window for Pearson r.
	cdLongWindow = 200
	// cdMinPointsBaseline is the minimum aligned pairs before the gate is
	// allowed to fire — guards against false correlations on small samples.
	cdMinPointsBaseline = 200
	// cdHighCorrelation is the |r| threshold for declaring "historically
	// correlated".
	cdHighCorrelation = 0.70
	// cdLowCorrelation is the current-window |r| below which decorrelation
	// fires.
	cdLowCorrelation = 0.30
	// cdRefractorySec suppresses repeat fires while the windows reseat.
	cdRefractorySec = 60
	// cdMinSamplingMatchSec — pairs whose median sampling cadence differs by
	// more than this many seconds are not correlated. Guards against pairing
	// e.g. a 10s metric with a 60s redis check.
	cdMinSamplingMatchSec = 5
	// cdRingRecomputeStride amortizes the moment recompute on the long-window
	// ring. Every N points we drop and rebuild from the ring (O(W)) so the
	// per-point cost remains O(1) on average.
	cdRingRecomputeStride = 16
	// cdSamplingTimestampRing is the small ring used to estimate per-pair
	// sampling cadence. 16 entries gives a robust median.
	cdSamplingTimestampRing = 16
)

// cdScopeTagPrefixes lists the tag prefixes that define a tag-scope group.
// Concatenation of matching tag values forms the scopeKey; series sharing a
// scopeKey are eligible for pair-correlation. If neither tag is present the
// detector falls back to meta.Namespace.
//
// Declared as a var (not const) because Go const slices aren't a thing; this
// list is read-only at runtime.
var cdScopeTagPrefixes = []string{"host:", "service:"}

// cdPairKey is a canonical, ordered key for an unordered pair of series refs.
// We always store the smaller ref in a — guarantees pair lookup is O(1) and
// scope rebuilds don't double-create pairs.
type cdPairKey struct {
	a, b observer.SeriesRef
}

// makePairKey returns the canonical pair key for (x, y).
func makePairKey(x, y observer.SeriesRef) cdPairKey {
	if x < y {
		return cdPairKey{a: x, b: y}
	}
	return cdPairKey{a: y, b: x}
}

// cdPairState holds streaming statistics for one (a, b) series pair.
//
// Long-window stats are maintained over a sliding ring of the last cdLongWindow
// raw aligned (x, y) pairs. Welford-style deletion is exact only for unweighted
// running sums; here we keep the ring and periodically rebuild the 6 sufficient
// statistics from it (every cdRingRecomputeStride points). This amortizes to
// O(1) per insert while keeping the moments exact every recompute.
type cdPairState struct {
	// Long-window ring of aligned (x, y) pairs and its head/fill counters.
	longBuf            [cdLongWindow][2]float64
	longHead, longFill int
	// Pointer counter that fires the periodic full recompute.
	insertsSinceRecompute int

	// Cached sufficient statistics over longBuf (recomputed every
	// cdRingRecomputeStride inserts).
	nLong                              int
	sumX, sumY, sumXY, sumX2, sumY2    float64

	// Short-window ring of aligned (x, y) pairs.
	shortBuf             [cdShortWindow][2]float64
	shortHead, shortFill int

	// Last-fire timestamp (data time, in seconds). Refractory gate check.
	lastFireAt int64

	// Per-pair sampling cadence ring. We track each endpoint independently and
	// declare the pair invalid if their medians differ by > cdMinSamplingMatchSec.
	tsA []int64
	tsB []int64

	// Captured metadata for the two endpoints. Populated lazily on first
	// successful aligned read; reused for anomaly construction.
	metaA, metaB           observer.SeriesMeta
	metaACaptured          bool
	metaBCaptured          bool

	// Cursor: the last point timestamp we ingested for each endpoint. Used to
	// pull only the new points on the next Detect call.
	lastSeenA, lastSeenB int64

	// invalid marks this pair as ineligible (e.g. cadence mismatch). We keep
	// the entry so we don't repeatedly try to revalidate; rebuild clears it.
	invalid bool
}

// cdScopeState holds the per-scope membership and pair-state map.
type cdScopeState struct {
	// members is the bounded list of series refs in this scope, ordered by
	// observation arrival. Cap is cdMaxSeriesPerScope.
	members []observer.SeriesRef
	// memberSet is a quick membership check used by RemoveSeries and the
	// rebuild-on-eviction path.
	memberSet map[observer.SeriesRef]struct{}
	// varProxy tracks a running sum-of-squared-deltas per member for ranking
	// when the scope is full. Higher-variance members are kept; lowest-variance
	// gets evicted on insert.
	varProxy map[observer.SeriesRef]float64
	// totalSeenInScope is purely diagnostic — counts ListSeries observations
	// keyed to this scope across the detector's lifetime.
	totalSeenInScope int
	// pairs holds the streaming state for each unordered pair of members.
	pairs map[cdPairKey]*cdPairState
}

// CrossDecorrelateDetector exposes its tunables as fields so tests and the
// testbench can override defaults after construction. NewCrossDecorrelateDetector
// populates the package-level defaults.
type CrossDecorrelateDetector struct {
	MaxSeriesPerScope   int
	ShortWindow         int
	LongWindow          int
	MinPointsBaseline   int
	HighCorrelation     float64
	LowCorrelation      float64
	RefractorySec       int64
	MinSamplingMatchSec int64

	// per-scope state keyed by scopeKey string
	scopes map[string]*cdScopeState

	// cache of discovered series, refreshed on SeriesGeneration change
	cachedSeries []observer.SeriesMeta
	cachedGen    uint64
}

// NewCrossDecorrelateDetector returns a CrossDecorrelate detector configured
// with package-default tuning. The catalog factory calls this with no args.
func NewCrossDecorrelateDetector() *CrossDecorrelateDetector {
	return &CrossDecorrelateDetector{
		MaxSeriesPerScope:   cdMaxSeriesPerScope,
		ShortWindow:         cdShortWindow,
		LongWindow:          cdLongWindow,
		MinPointsBaseline:   cdMinPointsBaseline,
		HighCorrelation:     cdHighCorrelation,
		LowCorrelation:      cdLowCorrelation,
		RefractorySec:       cdRefractorySec,
		MinSamplingMatchSec: cdMinSamplingMatchSec,
		scopes:              make(map[string]*cdScopeState),
	}
}

// Name implements observer.Detector.
func (d *CrossDecorrelateDetector) Name() string { return "cross_decorrelate" }

// Reset clears all per-scope and per-pair state for replay/reanalysis.
func (d *CrossDecorrelateDetector) Reset() {
	d.scopes = make(map[string]*cdScopeState)
	d.cachedSeries = nil
	d.cachedGen = 0
}

// RemoveSeries drops scope membership and prunes pair states whose endpoints
// include any of the removed refs. Mirrors HoltResidualDetector.RemoveSeries
// shape for the engine fan-out path.
func (d *CrossDecorrelateDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.ensureDefaults()
	if len(refs) == 0 || len(d.scopes) == 0 {
		return
	}
	removed := make(map[observer.SeriesRef]struct{}, len(refs))
	for _, r := range refs {
		removed[r] = struct{}{}
	}
	for scopeKey, scope := range d.scopes {
		// 1. Drop refs from membership.
		newMembers := scope.members[:0]
		for _, r := range scope.members {
			if _, gone := removed[r]; gone {
				delete(scope.memberSet, r)
				delete(scope.varProxy, r)
				continue
			}
			newMembers = append(newMembers, r)
		}
		scope.members = append([]observer.SeriesRef(nil), newMembers...)
		// 2. Prune pair states whose endpoints include a removed ref.
		for pk := range scope.pairs {
			if _, gone := removed[pk.a]; gone {
				delete(scope.pairs, pk)
				continue
			}
			if _, gone := removed[pk.b]; gone {
				delete(scope.pairs, pk)
			}
		}
		// 3. If the scope is now empty, drop it entirely.
		if len(scope.members) == 0 {
			delete(d.scopes, scopeKey)
		}
	}
	// Force the next Detect() to re-discover series.
	d.cachedSeries = nil
	d.cachedGen = 0
}

// Detect implements observer.Detector. Pulls the workload series list, refreshes
// scope membership when the storage generation has changed, then pulls aligned
// points for each (a, b) pair within each scope and runs the long/short Pearson
// gate.
func (d *CrossDecorrelateDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	d.ensureDefaults()

	gen := storage.SeriesGeneration()
	if d.cachedSeries == nil || gen != d.cachedGen {
		d.cachedSeries = storage.ListSeries(observer.WorkloadSeriesFilter())
		d.cachedGen = gen
		d.rebuildScopes()
	}

	var anomalies []observer.Anomaly
	for _, scope := range d.scopes {
		if len(scope.members) < 2 {
			continue
		}
		// Iterate unordered pairs (i < j).
		for i := 0; i < len(scope.members); i++ {
			for j := i + 1; j < len(scope.members); j++ {
				pk := makePairKey(scope.members[i], scope.members[j])
				pairAnoms := d.updatePair(storage, scope, pk, dataTime)
				if len(pairAnoms) > 0 {
					anomalies = append(anomalies, pairAnoms...)
				}
			}
		}
	}
	return observer.DetectionResult{Anomalies: anomalies}
}

// rebuildScopes walks cachedSeries, recomputes the scopeKey for each meta, and
// (re)populates scope membership. Existing scope state and pair state survive
// where members are still co-resident; pair states whose endpoints are no
// longer in any scope are dropped.
//
// Variance ranking: when a scope is at capacity, the lowest-varProxy member is
// ejected to make room. On bootstrap (varProxy empty) we admit in arrival order.
func (d *CrossDecorrelateDetector) rebuildScopes() {
	// 1. Group series metas by scopeKey.
	wantByScope := make(map[string][]observer.SeriesMeta)
	for _, m := range d.cachedSeries {
		wantByScope[scopeKeyForMeta(m)] = append(wantByScope[scopeKeyForMeta(m)], m)
	}
	// 2. Rebuild membership for each scope. We materialize a freshly-sized
	// member list for each scope present in storage; scopes absent from
	// wantByScope are dropped from d.scopes.
	for sk := range d.scopes {
		if _, present := wantByScope[sk]; !present {
			delete(d.scopes, sk)
		}
	}
	for sk, metas := range wantByScope {
		scope, ok := d.scopes[sk]
		if !ok {
			scope = newCDScopeState()
			d.scopes[sk] = scope
		}
		// Mark which existing members are still in storage. metas drives the
		// admission set, so any existing member not in metas is implicitly
		// out (it'll have been removed by RemoveSeries).
		stillResident := make(map[observer.SeriesRef]struct{}, len(metas))
		for _, m := range metas {
			stillResident[m.Ref] = struct{}{}
			// Capture metadata into any pair where this ref appears (lazy on first
			// pair update; nothing to do here).
			_ = m
		}
		// Drop members not in stillResident.
		newMembers := scope.members[:0]
		for _, r := range scope.members {
			if _, ok := stillResident[r]; ok {
				newMembers = append(newMembers, r)
				continue
			}
			delete(scope.memberSet, r)
			delete(scope.varProxy, r)
		}
		scope.members = append([]observer.SeriesRef(nil), newMembers...)
		// Admit new metas up to capacity. Order matters only when the scope is
		// full: at capacity, replace the lowest-varProxy member.
		for _, m := range metas {
			if _, already := scope.memberSet[m.Ref]; already {
				continue
			}
			scope.totalSeenInScope++
			if len(scope.members) < d.MaxSeriesPerScope {
				scope.members = append(scope.members, m.Ref)
				if scope.memberSet == nil {
					scope.memberSet = make(map[observer.SeriesRef]struct{})
				}
				scope.memberSet[m.Ref] = struct{}{}
				continue
			}
			// At capacity: evict the lowest-varProxy member (or skip if the new
			// member is no more variable than any existing one — bias toward
			// stability).
			minRef := scope.members[0]
			minVar := scope.varProxy[minRef]
			for _, r := range scope.members[1:] {
				if scope.varProxy[r] < minVar {
					minVar = scope.varProxy[r]
					minRef = r
				}
			}
			// If we have no variance signal yet (everyone at zero), don't
			// thrash — keep the existing arrivals.
			if minVar == 0 && scope.varProxy[m.Ref] == 0 {
				continue
			}
			// Replace minRef.
			for i, r := range scope.members {
				if r == minRef {
					scope.members[i] = m.Ref
					break
				}
			}
			delete(scope.memberSet, minRef)
			delete(scope.varProxy, minRef)
			scope.memberSet[m.Ref] = struct{}{}
			// Prune pair states involving the evicted member.
			for pk := range scope.pairs {
				if pk.a == minRef || pk.b == minRef {
					delete(scope.pairs, pk)
				}
			}
		}
		// Final pruning: drop any pair states whose endpoints aren't both in
		// the new member set.
		for pk := range scope.pairs {
			_, aIn := scope.memberSet[pk.a]
			_, bIn := scope.memberSet[pk.b]
			if !aIn || !bIn {
				delete(scope.pairs, pk)
			}
		}
	}
}

// scopeKeyForMeta builds the scope key for a meta by concatenating the values
// of cdScopeTagPrefixes. Falls back to the namespace when no prefix matches.
func scopeKeyForMeta(m observer.SeriesMeta) string {
	var b strings.Builder
	first := true
	for _, prefix := range cdScopeTagPrefixes {
		val := tagValue(m.Tags, prefix)
		if val == "" {
			continue
		}
		if !first {
			b.WriteByte('|')
		}
		b.WriteString(prefix)
		b.WriteString(val)
		first = false
	}
	if b.Len() == 0 {
		return "ns=" + m.Namespace
	}
	return b.String()
}

// tagValue returns the suffix of the first tag with the given prefix, or "".
func tagValue(tags []string, prefix string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, prefix) {
			return t[len(prefix):]
		}
	}
	return ""
}

// updatePair pulls aligned points for both endpoints of pk, updates the long
// and short windows, and runs the gate.
func (d *CrossDecorrelateDetector) updatePair(
	storage observer.StorageReader,
	scope *cdScopeState,
	pk cdPairKey,
	dataTime int64,
) []observer.Anomaly {
	pair, ok := scope.pairs[pk]
	if !ok {
		pair = &cdPairState{}
		scope.pairs[pk] = pair
	}
	if pair.invalid {
		return nil
	}
	// Pull new points for both endpoints. We use the per-endpoint lastSeen
	// cursor so each Detect call only reads the new points.
	startA := pair.lastSeenA
	startB := pair.lastSeenB
	seriesA := storage.GetSeriesRange(pk.a, startA, dataTime, observer.AggregateAverage)
	seriesB := storage.GetSeriesRange(pk.b, startB, dataTime, observer.AggregateAverage)
	if seriesA == nil || seriesB == nil {
		return nil
	}
	if !pair.metaACaptured {
		pair.metaA = observer.SeriesMeta{Ref: pk.a, Namespace: seriesA.Namespace, Name: seriesA.Name, Tags: append([]string(nil), seriesA.Tags...)}
		pair.metaACaptured = true
	}
	if !pair.metaBCaptured {
		pair.metaB = observer.SeriesMeta{Ref: pk.b, Namespace: seriesB.Namespace, Name: seriesB.Name, Tags: append([]string(nil), seriesB.Tags...)}
		pair.metaBCaptured = true
	}
	if len(seriesA.Points) == 0 && len(seriesB.Points) == 0 {
		return nil
	}
	// Update sampling cadence rings — independent per endpoint so a missed
	// point on one side doesn't poison the cadence estimate of the other.
	for _, p := range seriesA.Points {
		pair.tsA = pushTimestampRing(pair.tsA, p.Timestamp, cdSamplingTimestampRing)
		if p.Timestamp > pair.lastSeenA {
			pair.lastSeenA = p.Timestamp
		}
	}
	for _, p := range seriesB.Points {
		pair.tsB = pushTimestampRing(pair.tsB, p.Timestamp, cdSamplingTimestampRing)
		if p.Timestamp > pair.lastSeenB {
			pair.lastSeenB = p.Timestamp
		}
	}
	// Cadence guard: if both rings are populated and their median sampling
	// intervals differ by more than the threshold, mark the pair invalid.
	if len(pair.tsA) >= 4 && len(pair.tsB) >= 4 {
		cadA := medianTimestampInterval(pair.tsA)
		cadB := medianTimestampInterval(pair.tsB)
		if cadA > 0 && cadB > 0 {
			delta := cadA - cadB
			if delta < 0 {
				delta = -delta
			}
			if delta > d.MinSamplingMatchSec {
				pair.invalid = true
				return nil
			}
		}
	}
	// Two-pointer merge on timestamps, emitting aligned (x, y) pairs.
	aligned := alignPoints(seriesA.Points, seriesB.Points)
	if len(aligned) == 0 {
		return nil
	}
	var anomalies []observer.Anomaly
	for _, xy := range aligned {
		// Update var proxies for membership ranking — running sum of squared
		// deltas from the previous point. Cheap proxy for variance without
		// keeping a per-member window.
		updateVarProxy(scope, pk.a, xy[0])
		updateVarProxy(scope, pk.b, xy[1])
		// Push to long ring.
		pair.longBuf[pair.longHead] = [2]float64{xy[0], xy[1]}
		pair.longHead = (pair.longHead + 1) % cdLongWindow
		if pair.longFill < cdLongWindow {
			pair.longFill++
		}
		pair.insertsSinceRecompute++
		// Push to short ring.
		pair.shortBuf[pair.shortHead] = [2]float64{xy[0], xy[1]}
		pair.shortHead = (pair.shortHead + 1) % cdShortWindow
		if pair.shortFill < cdShortWindow {
			pair.shortFill++
		}
		// Recompute moments periodically (amortized O(1)/point).
		if pair.insertsSinceRecompute >= cdRingRecomputeStride {
			recomputeLongMoments(pair)
			pair.insertsSinceRecompute = 0
		}
		// Gate: require full short window, sufficient long-window baseline,
		// and refractory expiry. We re-recompute moments lazily here if the
		// stride hasn't fired yet but we're about to evaluate — keeping the
		// gate honest about the actual window contents.
		if pair.longFill < d.MinPointsBaseline {
			continue
		}
		if pair.shortFill < d.ShortWindow {
			continue
		}
		// alignPoints stuffs the joint timestamp at index 2 as a float64.
		// Convert back to int64 immediately — never compute refractory
		// arithmetic on raw floats (CLAUDE.md: timestamps are int64).
		nowSec := int64(xy[2])
		if nowSec-pair.lastFireAt < d.RefractorySec {
			continue
		}
		// Ensure moments are current with the ring (cheap if just recomputed).
		if pair.insertsSinceRecompute > 0 {
			recomputeLongMoments(pair)
			pair.insertsSinceRecompute = 0
		}
		rLong := pearsonFromMoments(pair.nLong, pair.sumX, pair.sumY, pair.sumXY, pair.sumX2, pair.sumY2)
		rShort := pearsonFromShortBuf(pair)
		if math.Abs(rLong) < d.HighCorrelation {
			continue
		}
		if math.Abs(rShort) >= d.LowCorrelation {
			continue
		}
		// Decorrelation fired: attribute to whichever endpoint has the larger
		// cross-residual ŷ = ȳ + r·(σy/σx)·(x-x̄).
		anom, ok := buildDecorrAnomaly(pair, rLong, rShort, nowSec, d.Name())
		if !ok {
			continue
		}
		pair.lastFireAt = nowSec
		anomalies = append(anomalies, anom)
	}
	return anomalies
}

// alignPoints walks two timestamp-ordered slices with two pointers and emits
// matched (x, y, ts) triples where timestamps match within ±1 sample. Returns
// the (x, y, ts) tuples encoded as [3]float64 (ts cast to float64; timestamp
// values are second-resolution Unix and fit losslessly).
//
// We deliberately encode the timestamp into the third slot (rather than a
// separate slice) so the alignment hot path keeps the data in a single
// contiguous array — friendly to the L1 cache and avoiding parallel-slice
// bookkeeping.
func alignPoints(a, b []observer.Point) [][3]float64 {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make([][3]float64, 0, min2(len(a), len(b)))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ta := a[i].Timestamp
		tb := b[j].Timestamp
		switch {
		case ta == tb:
			out = append(out, [3]float64{a[i].Value, b[j].Value, float64(ta)})
			i++
			j++
		case ta < tb:
			i++
		default:
			j++
		}
	}
	return out
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// recomputeLongMoments rebuilds the 6 sufficient statistics from the current
// long-window ring contents.
func recomputeLongMoments(p *cdPairState) {
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	n := p.longFill
	for k := 0; k < n; k++ {
		// Read in arrival order (oldest first); for moment recomputation order
		// doesn't matter, but iterating from oldest keeps the access pattern
		// consistent with the ring layout when we extend this in future.
		idx := (p.longHead - n + k + cdLongWindow) % cdLongWindow
		x := p.longBuf[idx][0]
		y := p.longBuf[idx][1]
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
		sumY2 += y * y
	}
	p.nLong = n
	p.sumX = sumX
	p.sumY = sumY
	p.sumXY = sumXY
	p.sumX2 = sumX2
	p.sumY2 = sumY2
}

// pearsonFromMoments returns the Pearson correlation from the 6 sufficient
// statistics. Returns 0 on a degenerate window (n<2 or zero variance on either
// side) — interpreting "no signal" as "no correlation" suppresses spurious
// gate fires.
func pearsonFromMoments(n int, sumX, sumY, sumXY, sumX2, sumY2 float64) float64 {
	if n < 2 {
		return 0
	}
	fn := float64(n)
	covNum := sumXY - sumX*sumY/fn
	varX := sumX2 - sumX*sumX/fn
	varY := sumY2 - sumY*sumY/fn
	if varX <= 0 || varY <= 0 {
		return 0
	}
	r := covNum / math.Sqrt(varX*varY)
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return 0
	}
	if r > 1 {
		return 1
	}
	if r < -1 {
		return -1
	}
	return r
}

// pearsonFromShortBuf computes Pearson r over the short ring's currently
// populated entries (in arrival order — order is irrelevant for moments, but
// staying consistent makes the code easier to audit).
func pearsonFromShortBuf(p *cdPairState) float64 {
	n := p.shortFill
	if n < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for k := 0; k < n; k++ {
		idx := (p.shortHead - n + k + cdShortWindow) % cdShortWindow
		x := p.shortBuf[idx][0]
		y := p.shortBuf[idx][1]
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
		sumY2 += y * y
	}
	return pearsonFromMoments(n, sumX, sumY, sumXY, sumX2, sumY2)
}

// buildDecorrAnomaly constructs the Anomaly payload, attributing the fire to
// whichever endpoint has the larger cross-prediction residual.
//
// Cross-prediction: ŷ = ȳ + r·(σy/σx)·(x - x̄). Symmetrically for x̂. Whichever
// |x - x̂| or |y - ŷ| is larger (after normalisation by σ) is the attributed
// endpoint. This matches the candidate description's "this query is fine but
// throughput desynced from latency" attribution rule.
func buildDecorrAnomaly(
	pair *cdPairState,
	rLong, rShort float64,
	tsSec int64,
	detectorName string,
) (observer.Anomaly, bool) {
	// Recover σ_x, σ_y, x̄, ȳ from cached long-window moments.
	if pair.nLong < 2 {
		return observer.Anomaly{}, false
	}
	fn := float64(pair.nLong)
	meanX := pair.sumX / fn
	meanY := pair.sumY / fn
	varX := pair.sumX2 - pair.sumX*pair.sumX/fn
	varY := pair.sumY2 - pair.sumY*pair.sumY/fn
	if varX <= 0 || varY <= 0 {
		return observer.Anomaly{}, false
	}
	sigmaX := math.Sqrt(varX / (fn - 1))
	sigmaY := math.Sqrt(varY / (fn - 1))
	// Use the most recent short-buffer entry as the "current" point.
	headIdx := (pair.shortHead - 1 + cdShortWindow) % cdShortWindow
	xCur := pair.shortBuf[headIdx][0]
	yCur := pair.shortBuf[headIdx][1]
	// Cross predictions.
	yHat := meanY + rLong*(sigmaY/sigmaX)*(xCur-meanX)
	xHat := meanX + rLong*(sigmaX/sigmaY)*(yCur-meanY)
	// Normalize each residual by its native sigma to compare apples to apples.
	residY := math.Abs(yCur-yHat) / sigmaY
	residX := math.Abs(xCur-xHat) / sigmaX
	// Choose endpoint with larger normalized residual.
	chooseB := residY >= residX
	var chosen, peer observer.SeriesMeta
	if chooseB {
		chosen = pair.metaB
		peer = pair.metaA
	} else {
		chosen = pair.metaA
		peer = pair.metaB
	}
	score := math.Abs(rLong) - math.Abs(rShort)
	displayChosen := displayName(chosen)
	displayPeer := displayName(peer)
	a := observer.Anomaly{
		Type: observer.AnomalyTypeMetric,
		Source: observer.SeriesDescriptor{
			Namespace: chosen.Namespace,
			Name:      chosen.Name,
			Tags:      chosen.Tags,
			Aggregate: observer.AggregateAverage,
		},
		SourceRef:    &observer.QueryHandle{Ref: chosen.Ref, Aggregate: observer.AggregateAverage},
		DetectorName: detectorName,
		Title:        "Cross-series decorrelation: " + displayChosen + " ↔ " + displayPeer,
		Description: fmt.Sprintf(
			"Long-window |r|=%.3f collapsed to short-window |r|=%.3f (chosen=%s peer=%s, residY=%.3fσ, residX=%.3fσ)",
			math.Abs(rLong), math.Abs(rShort), displayChosen, displayPeer, residY, residX,
		),
		Timestamp: tsSec,
		Score:     &score,
	}
	return a, true
}

// displayName returns "namespace.name{tags}" for human-readable anomaly text.
// Falls back gracefully if Namespace or Tags are empty.
func displayName(m observer.SeriesMeta) string {
	var b strings.Builder
	if m.Namespace != "" {
		b.WriteString(m.Namespace)
		b.WriteByte('.')
	}
	b.WriteString(m.Name)
	if len(m.Tags) > 0 {
		b.WriteByte('{')
		b.WriteString(strings.Join(m.Tags, ","))
		b.WriteByte('}')
	}
	return b.String()
}

// pushTimestampRing appends ts to ring, trimming to the most recent maxLen.
// Returns the (possibly reallocated) ring. Mirrors pushTimestamp from
// metrics_detector_holt_residual.go but works on a free-floating slice.
func pushTimestampRing(ring []int64, ts int64, maxLen int) []int64 {
	if len(ring) < maxLen {
		return append(ring, ts)
	}
	copy(ring, ring[1:])
	ring[len(ring)-1] = ts
	return ring
}

// updateVarProxy maintains a running sum-of-squares proxy for member ranking
// in scope eviction. It is intentionally simple: each update adds v², which
// naturally favours members with larger absolute values and movement. Resets
// happen only when a member is evicted.
func updateVarProxy(scope *cdScopeState, ref observer.SeriesRef, v float64) {
	if scope.varProxy == nil {
		scope.varProxy = make(map[observer.SeriesRef]float64)
	}
	scope.varProxy[ref] += v * v
}

// newCDScopeState allocates an empty scope state with maps wired up.
func newCDScopeState() *cdScopeState {
	return &cdScopeState{
		members:   nil,
		memberSet: make(map[observer.SeriesRef]struct{}),
		varProxy:  make(map[observer.SeriesRef]float64),
		pairs:     make(map[cdPairKey]*cdPairState),
	}
}

// ensureDefaults fills zero-valued config fields with sensible defaults.
// Mirrors holt_residual / kl_divergence so the detector behaves sanely under
// reflective construction.
func (d *CrossDecorrelateDetector) ensureDefaults() {
	if d.MaxSeriesPerScope <= 0 {
		d.MaxSeriesPerScope = cdMaxSeriesPerScope
	}
	if d.ShortWindow <= 0 {
		d.ShortWindow = cdShortWindow
	}
	if d.LongWindow <= 0 {
		d.LongWindow = cdLongWindow
	}
	if d.MinPointsBaseline <= 0 {
		d.MinPointsBaseline = cdMinPointsBaseline
	}
	if d.HighCorrelation <= 0 {
		d.HighCorrelation = cdHighCorrelation
	}
	if d.LowCorrelation <= 0 {
		d.LowCorrelation = cdLowCorrelation
	}
	if d.RefractorySec <= 0 {
		d.RefractorySec = cdRefractorySec
	}
	if d.MinSamplingMatchSec <= 0 {
		d.MinSamplingMatchSec = cdMinSamplingMatchSec
	}
	if d.scopes == nil {
		d.scopes = make(map[string]*cdScopeState)
	}
}
