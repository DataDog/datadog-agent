package aggregator

// Gauge stores and aggregates a gauge value
type Gauge struct {
	gauge     float64
	timestamp int64
}

func (g *Gauge) addSample(sample float64, timestamp int64) {
	g.gauge = sample
	g.timestamp = timestamp
}

func (g *Gauge) flush() (float64, int64) {
	return g.gauge, g.timestamp
}

// Counter stores and aggregates a counter values
type Counter struct {
	count     int
	timestamp int64
}

func (c *Counter) addSample(sample float64, timestamp int64) {
	// TODO
}
