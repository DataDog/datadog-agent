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

// NoSketchError is the error returned when not enough samples have been
//submitted to generate a sketch
type NoSketchError struct{}

func (e NoSketchError) Error() string {
	return "Not enough samples to generate sketches"
}

func marshalSketches(sketches []Sketch) []agentpayload.SketchPayload_Summary_Sketch {
	sketchesPayload := []agentpayload.SketchPayload_Summary_Sketch{}

	for _, s := range sketches {
		sketchesPayload = append(sketchesPayload,
			agentpayload.SketchPayload_Summary_Sketch{
				Ts:      s.Timestamp,
				N:       int64(s.Sketch.ValCount),
				Min:     s.Sketch.Min,
				Entries: marshalEntries(s.Sketch.Entries),
			})
	}
	return sketchesPayload
}

func unmarshalSketches(summarySketches []agentpayload.SketchPayload_Summary_Sketch) []Sketch {
	sketches := []Sketch{}
	for _, s := range summarySketches {
		sketches = append(sketches,
			Sketch{
				Timestamp: s.Ts,
				Sketch: QSketch{
					GKArray{Min: s.Min,
						ValCount: int(s.N),
						Entries:  unmarshalEntries(s.Entries),
						incoming: make([]float64, 0, int(1/EPSILON))}},
			})
	}
	return sketches

}

// MarshalSketchSeries serializes sketch series using protocol buffers
func MarshalSketchSeries(sketches []*SketchSeries) ([]byte, error) {
	payload := &agentpayload.SketchPayload{
		Summaries: []agentpayload.SketchPayload_Summary{},
		Metadata:  agentpayload.CommonMetadata{},
	}
	for _, s := range sketches {
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
