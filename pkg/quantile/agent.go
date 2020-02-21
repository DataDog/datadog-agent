package quantile

const (
	agentBufCap = 512
)

var agentConfig = Default()

// An Agent sketch is an insert optimized version of the sketch for use in the
// datadog-agent.
type Agent struct {
	Sketch   Sketch
	Buf      []Key
	CountBuf []KeyCount
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
	if len(a.Buf) != 0 {
		a.Sketch.insert(agentConfig, a.Buf)
		a.Buf = nil
	}

	if len(a.CountBuf) != 0 {
		a.Sketch.insertCounts(agentConfig, a.CountBuf)
		a.CountBuf = nil
	}
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

// InsertInterpolate linearly interpolates counts from lower (l) to upper (u)
func (a *Agent) InsertInterpolate(l float64, u float64, n uint) {
	keys := make([]Key, 0)
	for k := agentConfig.key(l); k <= agentConfig.key(u); k++ {
		keys = append(keys, k)
	}
	whatsLeft := n
	distance := u - l
	kStartIdx := 0
	lowerB := agentConfig.binLow(keys[kStartIdx])
	kEndIdx := 1
	for kEndIdx < len(keys) {
		upperB := agentConfig.binLow(keys[kEndIdx])
		// ((upperB - lowerB) / distance) is the ratio of the distance between the current buckets to the total distance
		// which tells us how much of the remaining value to put in this bucket
		kn := uint(((upperB - lowerB) / distance) * float64(n))
		if kn > 0 {
			a.Sketch.Basic.InsertN(lowerB, kn)
			a.CountBuf = append(a.CountBuf, KeyCount{k: keys[kStartIdx], n: kn})
			whatsLeft -= kn
			kStartIdx = kEndIdx
			lowerB = upperB
		}
		kEndIdx++
	}
	a.Sketch.Basic.InsertN(agentConfig.binLow(keys[kStartIdx]), whatsLeft)
	a.CountBuf = append(a.CountBuf, KeyCount{k: keys[kStartIdx], n: whatsLeft})
	a.flush()
}
