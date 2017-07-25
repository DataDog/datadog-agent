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

// SketchSeriesList represents a list of SketchSeries ready to be serialize
type SketchSeriesList []*SketchSeries

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

func marshalEntries(entries Entries) []agentpayload.SketchPayload_Summary_Sketch_Entry {
	entriesPayload := []agentpayload.SketchPayload_Summary_Sketch_Entry{}
	for _, e := range entries {
		entriesPayload = append(entriesPayload,
			agentpayload.SketchPayload_Summary_Sketch_Entry{
				V:     e.V,
				G:     int64(e.G),
				Delta: int64(e.Delta),
			})
	}
	return entriesPayload
}

func marshalSketches(sketches []Sketch) []agentpayload.SketchPayload_Summary_Sketch {
	sketchesPayload := []agentpayload.SketchPayload_Summary_Sketch{}

	for _, s := range sketches {
		sketchesPayload = append(sketchesPayload,
			agentpayload.SketchPayload_Summary_Sketch{
				Ts:      s.Timestamp,
				N:       int64(s.Sketch.Count),
				Min:     s.Sketch.Min,
				Max:     s.Sketch.Max,
				Avg:     s.Sketch.Avg,
				Sum:     s.Sketch.Sum,
				Entries: marshalEntries(s.Sketch.Entries),
			})
	}
	return sketchesPayload
}

// Marshal serializes sketch series using protocol buffers
func (sl SketchSeriesList) Marshal() ([]byte, error) {
	payload := &agentpayload.SketchPayload{
		Summaries: []agentpayload.SketchPayload_Summary{},
		Metadata:  agentpayload.CommonMetadata{},
	}
	for _, s := range sl {
		payload.Summaries = append(payload.Summaries,
			agentpayload.SketchPayload_Summary{
				Metric:   s.Name,
				Host:     s.Host,
				Sketches: marshalSketches(s.Sketches),
				Tags:     s.Tags,
			})
	}
	return proto.Marshal(payload)
}

// MarshalJSON serializes sketch series to JSON so it can be sent to
// v1 endpoints
func (sl SketchSeriesList) MarshalJSON() ([]byte, error) {
	data := map[string][]*SketchSeries{
		"sketch_series": sl,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}
