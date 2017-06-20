package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/percentile"
)

// Sketch represents a quantile sketch at a specific time
type Sketch struct {
	Timestamp int64   `json:"timestamp"`
	Sketch    QSketch `json:"qsketch"`
}

// SketchSerie holds an array of sketches.
type SketchSerie struct {
	Name       string   `json:"metric"`
	Tags       []string `json:"tags"`
	Host       string   `json:"host"`
	Interval   int64    `json:"interval"`
	Sketches   []Sketch `json:"sketches"`
	contextKey string
}

// QSketch is a wrapper around GKArray to make it easier if we want to try a
// different sketch algorithm
type QSketch struct {
	percentile.GKArray
}

// NewQSketch creates a new QSketch
func NewQSketch() QSketch {
	return QSketch{percentile.NewGKArray()}
}

// NoSketchError is the error returned when not enough samples have been
//submitted to generate a sketch
type NoSketchError struct{}

func (e NoSketchError) Error() string {
	return "Not enough samples to generate sketches"
}
