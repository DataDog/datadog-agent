package percentile

import (
	"bytes"
	"encoding/json"
)

// Sketch represents a quantile sketch at a specific time
type Sketch struct {
	Timestamp int64   `json:"timestamp"`
	Sketch    QSketch `json:"qsketch"`
}

// SketchSeries holds an array of sketches.
type SketchSeries struct {
	Name       string   `json:"metric"`
	Tags       []string `json:"tags"`
	Host       string   `json:"host"`
	Interval   int64    `json:"interval"`
	Sketches   []Sketch `json:"sketches"`
	ContextKey string   `json:"-"`
}

// QSketch is a wrapper around GKArray to make it easier if we want to try a
// different sketch algorithm
type QSketch struct {
	GKArray
}

// NewQSketch creates a new QSketch
func NewQSketch() QSketch {
	return QSketch{NewGKArray()}
}

// NoSketchError is the error returned when not enough samples have been
//submitted to generate a sketch
type NoSketchError struct{}

func (e NoSketchError) Error() string {
	return "Not enough samples to generate sketches"
}

// MarshalJSONSketchSeries serializes sketch series to JSON so it can be sent to
// v1 endpoints
func MarshalJSONSketchSeries(sketches []*SketchSeries) ([]byte, error) {
	data := map[string][]*SketchSeries{
		"sketch_series": sketches,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

//UnmarshalJSONSketchSeries deserializes sketch series from JSON
func UnmarshalJSONSketchSeries(b []byte) ([]*SketchSeries, error) {
	data := make(map[string][]*SketchSeries, 0)
	r := bytes.NewReader(b)
	err := json.NewDecoder(r).Decode(&data)
	if err != nil {
		return []*SketchSeries{}, err
	}
	return data["sketch_series"], nil
}
