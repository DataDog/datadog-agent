package serializer

import (
	"bytes"
	"encoding/json"

	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"
)

func marshalEntries(entries percentile.Entries) []agentpayload.SketchPayload_Summary_Sketch_Entry {
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

func marshalSketches(sketches []percentile.Sketch) []agentpayload.SketchPayload_Summary_Sketch {
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

// MarshalSketchSeries serializes sketch series using protocol buffers
func MarshalSketchSeries(sketches []*percentile.SketchSeries) ([]byte, string, error) {
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
	msg, err := proto.Marshal(payload)
	return msg, protobufContentType, err
}

// MarshalJSONSketchSeries serializes sketch series to JSON so it can be sent to
// v1 endpoints
func MarshalJSONSketchSeries(sketches []*percentile.SketchSeries) ([]byte, string, error) {
	data := map[string][]*percentile.SketchSeries{
		"sketch_series": sketches,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), jsonContentType, err
}
