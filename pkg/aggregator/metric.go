package aggregator

type Gauge struct {
	gauge float64
}

func (g *Gauge) addSample(sample float64) {
	g.gauge = sample
}

func (g *Gauge) flush() float64 {
	return g.gauge
}

type Counter struct {
	count int
}

func (c *Counter) addSample(sample float64) {
	// TODO
}
