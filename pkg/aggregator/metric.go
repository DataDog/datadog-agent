package aggregator

// Gauge stores and aggregates a gauge value
type Gauge struct {
	gauge float64
}

func (g *Gauge) addSample(sample float64) {
	g.gauge = sample
}

func (g *Gauge) flush() float64 {
	return g.gauge
}

// Counter stores and aggregates a counter values
type Counter struct {
	count int
}

func (c *Counter) addSample(sample float64) {
	// TODO
}
