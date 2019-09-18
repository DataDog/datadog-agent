package quantile

import "math"

const (
	agentBufCap = 512
)

var agentConfig = Default()

// An Agent sketch is an insert optimized version of the sketch for use in the
// datadog-agent.
type Agent struct {
	Sketch Sketch
	Buf    []Key
}

// IsEmpty returns true if the sketch is empty
func (a *Agent) IsEmpty() bool {
	return a.Sketch.Basic.Cnt == 0 && len(a.Buf) == 0
}

// Finish flushes any pending inserts and returns a deep copy of the sketch.
func (a *Agent) Finish() *Sketch {
	a.flush()

	if a.IsEmpty() {
		return nil
	}

	return a.Sketch.Copy()
}

// flush buffered values into the sketch.
func (a *Agent) flush() {
	if len(a.Buf) == 0 {
		return
	}

	a.Sketch.insert(agentConfig, a.Buf)
	a.Buf = nil
}

// Reset the agent sketch to the empty state.
func (a *Agent) Reset() {
	a.Sketch.Reset()
	a.Buf = nil // TODO: pool
}

// Insert v into the sketch.
func (a *Agent) Insert(v float64) {
	a.Sketch.Basic.Insert(v)

	a.Buf = append(a.Buf, agentConfig.key(v))
	if len(a.Buf) < agentBufCap {
		return
	}

	a.flush()
}

// InsertBucket inserts `count` points assuming a uniform distribution between `low` and `high`.
// TODO: Summary statistics.
func (a *Agent) InsertBucket(low, high float64, count uint) {
	lowKey := agentConfig.key(low)
	highKey := agentConfig.key(high)

	var bins []bin
	if lowKey == highKey { // Everything goes in the one sketch bucket.
		bins = appendSafe(bins, lowKey, int(count))
	} else if lowKey+1 == highKey { // Balance between two sketch buckets.
		bound := agentConfig.f64(highKey)
		// Add points to the lower key based on the amount the two sketch buckets overlap with the
		// provided bucket.
		lowPoints := int(math.Round((bound - low) / (high - low) * float64(count)))
		bins = appendSafe(bins, lowKey, lowPoints)
		bins = appendSafe(bins, highKey, int(count)-lowPoints)
	} else { // More than two sketch buckets.
		// Add the points for the bottom bucket.
		ratio := float64(count) / (high - low)
		firstPow := agentConfig.f64(lowKey + 1)
		lowPoints := int(math.Round((firstPow - low) * ratio))
		bins = appendSafe(bins, lowKey, lowPoints)

		// Add the middle points.
		addedPoints := lowPoints
		bucketLen := firstPow - agentConfig.f64(lowKey)
		for key := lowKey + 1; key < highKey; key++ {
			bucketLen = agentConfig.gamma.v * bucketLen
			points := int(math.Round(bucketLen * ratio))
			bins = appendSafe(bins, key, points)
			addedPoints += points
		}

		// Add the remaining points.
		bins = appendSafe(bins, highKey, int(count)-addedPoints)
	}

	sparse := &sparseStore{count: int(count), bins: bins}
	a.Sketch.merge(agentConfig, sparse)
}

// InsertN inserts v, n times into the sketch.
func (a *Agent) InsertN(v float64, n uint) {
	a.Sketch.Basic.InsertN(v, n)

	for i := 0; i < int(n); i++ {
		a.Buf = append(a.Buf, agentConfig.key(v))
	}
	if len(a.Buf) < agentBufCap {
		return
	}

	a.flush()
}
