package percentile

import (
	"bytes"
	"encoding/json"

	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/gogen"
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

// Add a value to the qsketch
func (q QSketch) Add(v float64) QSketch {
	return QSketch{GKArray: q.GKArray.Add(v)}
}

// Compress the qsketch
func (q QSketch) Compress() QSketch {
	return QSketch{GKArray: q.GKArray.Compress()}
}

// NoSketchError is the error returned when not enough samples have been
//submitted to generate a sketch
type NoSketchError struct{}

func (e NoSketchError) Error() string {
	return "Not enough samples to generate sketches"
}

// UnmarshalSketchSeries deserializes a protobuf byte array into sketch series
func UnmarshalSketchSeries(payload []byte) ([]*SketchSeries, agentpayload.CommonMetadata, error) {
	sketches := []*SketchSeries{}
	decodedPayload := &agentpayload.SketchPayload{}
	err := proto.Unmarshal(payload, decodedPayload)
	if err != nil {
		return sketches, agentpayload.CommonMetadata{}, err
	}
	for _, s := range decodedPayload.Summaries {
		sketches = append(sketches,
			&SketchSeries{
				Name:     s.Metric,
				Tags:     s.Tags,
				Host:     s.Host,
				Sketches: unmarshalSketches(s.Sketches),
			})
	}
	return sketches, decodedPayload.Metadata, err
}

func unmarshalSketches(summarySketches []agentpayload.SketchPayload_Summary_Sketch) []Sketch {
	sketches := []Sketch{}
	for _, s := range summarySketches {
		sketches = append(sketches,
			Sketch{
				Timestamp: s.Ts,
				Sketch: QSketch{
					GKArray{Min: s.Min,
						Count:    int(s.N),
						Max:      s.Max,
						Avg:      s.Avg,
						Sum:      s.Sum,
						Entries:  unmarshalEntries(s.Entries),
						incoming: make([]float64, 0, int(1/EPSILON))}},
			})
	}
	return sketches
}

// UnmarshalJSONSketchSeries deserializes sketch series from JSON
func UnmarshalJSONSketchSeries(b []byte) ([]*SketchSeries, error) {
	data := make(map[string][]*SketchSeries, 0)
	r := bytes.NewReader(b)
	err := json.NewDecoder(r).Decode(&data)
	if err != nil {
		return []*SketchSeries{}, err
	}
	return data["sketch_series"], nil
}
