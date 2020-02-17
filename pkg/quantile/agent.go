package quantile

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
