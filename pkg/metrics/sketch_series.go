package metrics

import (
	"bytes"
	"encoding/json"

	"github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// A SketchSeries is a timeseries of quantile sketches.
type SketchSeries struct {
	Name       string          `json:"metric"`
	Tags       []string        `json:"tags"`
	Host       string          `json:"host"`
	Interval   int64           `json:"interval"`
	Points     []SketchPoint   `json:"points"`
	ContextKey ckey.ContextKey `json:"-"`
}

// A SketchPoint represents a quantile sketch at a specific time
type SketchPoint struct {
	Sketch *quantile.Sketch `json:"sketch"`
	Ts     int64            `json:"ts"`
}

// A SketchSeriesList implements marshaler.Marshaler
type SketchSeriesList []SketchSeries

// MarshalJSON serializes sketch series to JSON.
func (sl SketchSeriesList) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing a SketchSeriesList
	type SketchSeriesAlias SketchSeriesList

	data := map[string][]SketchSeries{
		"sketches": SketchSeriesAlias(sl),
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// Marshal encodes this series list.
func (sl SketchSeriesList) Marshal() ([]byte, error) {
	pb := &gogen.SketchPayload{
		Sketches: make([]gogen.SketchPayload_Sketch, 0, len(sl)),
	}

	for _, ss := range sl {
		dsl := make([]gogen.SketchPayload_Sketch_Dogsketch, 0, len(ss.Points))

		for _, p := range ss.Points {
			b := p.Sketch.Basic
			k, n := p.Sketch.Cols()
			dsl = append(dsl, gogen.SketchPayload_Sketch_Dogsketch{
				Ts:  p.Ts,
				Cnt: b.Cnt,
				Min: b.Min,
				Max: b.Max,
				Avg: b.Avg,
				Sum: b.Sum,
				K:   k,
				N:   n,
			})
		}

		pb.Sketches = append(pb.Sketches, gogen.SketchPayload_Sketch{
			Metric:      ss.Name,
			Host:        ss.Host,
			Tags:        ss.Tags,
			Dogsketches: dsl,
		})
	}

	return pb.Marshal()
}

// SplitPayload breaks the payload into times number of pieces
func (sl SketchSeriesList) SplitPayload(times int) ([]marshaler.Marshaler, error) {
	// Only break it down as much as possible
	if len(sl) < times {
		times = len(sl)
	}
	splitPayloads := make([]marshaler.Marshaler, times)
	batchSize := len(sl) / times
	n := 0
	for i := 0; i < times; i++ {
		var end int
		// In many cases the batchSize is not perfect
		// so the last one will be a bit bigger or smaller than the others
		if i < times-1 {
			end = n + batchSize
		} else {
			end = len(sl)
		}
		newSL := sl[n:end]
		splitPayloads[i] = newSL
		n += batchSize
	}
	return splitPayloads, nil
}
