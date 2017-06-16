package aggregator

import (
	"fmt"
)

// Sketch represents a quantile sketch at a specific time
type Sketch struct {
	timestamp int64
	sketch    QSketch
}

// SketchSerie holds an array of sketches.
type SketchSerie struct {
	Name       string   `json:"metric"`
	Tags       []string `json:"tags"`
	Host       string   `json:"host"`
	DeviceName string   `json:"device_name"`
	Interval   int64    `json:"interval"`
	Sketches   []Sketch `json:"sketches"`
	contextKey string
}

// MarshalJSON returns a Sketch as an array of timestamp, percentile sketch pairs
func (s *Sketch) MarshalJSON() ([]byte, error) {
	sketchStr := fmt.Sprintf("[%v, [[", s.timestamp)
	for _, entry := range s.sketch.Entries {
		sketchStr += fmt.Sprintf("[%v, %v, %v],", entry.V, entry.G, entry.Delta)
	}
	// remove the last comma
	sketchStr = sketchStr[:len(sketchStr)-1]
	sketchStr += fmt.Sprintf("], %v]]", s.sketch.N)
	return []byte(sketchStr), nil
}

// NoSketchError is the error returned when not enough samples have been
//submitted to generate a sketch
type NoSketchError struct{}

func (e NoSketchError) Error() string {
	return "Not enough samples to generate sketches"
}
