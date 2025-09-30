// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

// Bound is a quantized value and its corresponding lower bound.
type Bound struct {
	// Key is the quantized value of the bound.
	Key Key
	Low float64 // Low is the lower bound of the range.
}

// DDSketchBinGenerator is a generator for DDSketch bounds.
type DDSketchBinGenerator struct {
	// Config is the configuration for the sketch.
	Config   *Config
	Bounds   []Bound
	BoundMap map[Key]*Bound
}

// NewDDSketchBinGeneratorForAgent creates a new DDSketchBinGenerator for the agent.
func NewDDSketchBinGeneratorForAgent() *DDSketchBinGenerator {
	return NewDDSketchBinGenerator(agentConfig)
}

// NewDDSketchBinGenerator creates a new DDSketchBinGenerator for the given config.
func NewDDSketchBinGenerator(config *Config) *DDSketchBinGenerator {
	dg := &DDSketchBinGenerator{
		Config:   config,
		Bounds:   make([]Bound, 0, defaultBinListSize),
		BoundMap: make(map[Key]*Bound, defaultBinLimit),
	}
	dg.generateBounds()
	return dg
}

// GetBounds returns the bounds for the DDSketchBinGenerator.
func (g *DDSketchBinGenerator) GetBounds() []Bound {
	return g.Bounds
}

func (g *DDSketchBinGenerator) generateBounds() {
	half := g.Config.binLimit / 2
	for i := -half; i < half; i++ {
		key := Key(i)
		low := g.Config.binLow(key)
		b := Bound{Key: key, Low: low}
		g.Bounds = append(g.Bounds, b)
		g.BoundMap[key] = &b
	}
}

// GetBound returns the bound for the given key.
func (g *DDSketchBinGenerator) GetBound(key Key) (*Bound, bool) {
	bound, ok := g.BoundMap[key]
	if !ok {
		return nil, false
	}
	return bound, true
}

// GetKeyForValue returns the key for the given value.
func (g *DDSketchBinGenerator) GetKeyForValue(value float64) Key {
	return g.Config.key(value)
}
