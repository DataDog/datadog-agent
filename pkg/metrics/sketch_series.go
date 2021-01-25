package metrics

import (
	"bytes"
	"encoding/json"

	"github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/common"
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

type SketchSeriesList struct {
	SketchSeries []SketchSeries
	buf          *[]byte
}

func NewSketchSeriesList(sl []SketchSeries) *SketchSeriesList {
	b := make([]byte, 1024)
	return &SketchSeriesList{
		SketchSeries: sl,
		buf:          &b,
	}
}

// var mu sync.Mutex
// var buf = make([]byte, 1024)

// var playloadBuf = make([]gogen.SketchPayload_Sketch, 100)

// A SketchSeriesList implements marshaler.Marshaler
// type SketchSeriesList []SketchSeries

// MarshalJSON serializes sketch series to JSON.
// Quite slow, but hopefully this method is called only in the `agent check` command
func (sl SketchSeriesList) MarshalJSON() ([]byte, error) {
	// We use this function to customize generated JSON
	// This function, only used when displaying `bins`, is especially slow
	// As `StructToMap` function is using reflection to return a generic map[string]interface{}
	customSketchSeries := func(srcSl []SketchSeries) []interface{} {
		dstSl := make([]interface{}, 0, len(srcSl))

		for _, ss := range srcSl {
			ssMap := common.StructToMap(ss)
			for i, sketchPoint := range ss.Points {
				if sketchPoint.Sketch != nil {
					sketch := ssMap["points"].([]interface{})[i].(map[string]interface{})
					count, bins := sketchPoint.Sketch.GetRawBins()
					sketch["binsCount"] = count
					sketch["bins"] = bins
				}
			}

			dstSl = append(dstSl, ssMap)
		}

		return dstSl
	}

	// use an alias to avoid infinite recursion while serializing a SketchSeriesList
	if config.Datadog.GetBool("cmd.check.fullsketches") {
		data := map[string]interface{}{
			"sketches": customSketchSeries(sl.SketchSeries),
		}

		reqBody := &bytes.Buffer{}
		err := json.NewEncoder(reqBody).Encode(data)
		return reqBody.Bytes(), err
	}

	type SketchSeriesAlias []SketchSeries
	data := map[string]SketchSeriesAlias{
		"sketches": SketchSeriesAlias(sl.SketchSeries),
	}

	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// Marshal encodes this series list.
func (sl SketchSeriesList) Marshal() ([]byte, error) {
	// if len(sl) > cap(playloadBuf) {
	// 	playloadBuf = make([]gogen.SketchPayload_Sketch, 0, len(sl))
	// }
	// pb := &gogen.SketchPayload{
	// 	Sketches: playloadBuf,
	// }

	pb := &gogen.SketchPayload{
		Sketches: make([]gogen.SketchPayload_Sketch, 0, len(sl.SketchSeries)),
	}

	for _, ss := range sl.SketchSeries {
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
	if pb.Size() > cap(sl.buf) {
		//sl.buf = make([]byte, pb.Size())
		sl.buf = append(sl.buf, make([]byte, pb.Size()-cap(sl.buf))...)
	}
	n, err := pb.MarshalTo(sl.buf)
	return sl.buf[:n], err

	// if pb.Size() > cap(buf) {
	// 	buf = make([]byte, pb.Size())
	// }
	// n, err := pb.MarshalTo(buf)
	// return buf[:n], err

	// return pb.Marshal()
}

// SplitPayload breaks the payload into times number of pieces
func (sl SketchSeriesList) SplitPayload(times int) ([]marshaler.Marshaler, error) {
	// Only break it down as much as possible
	if len(sl.SketchSeries) < times {
		times = len(sl.SketchSeries)
	}
	splitPayloads := make([]marshaler.Marshaler, times)
	batchSize := len(sl.SketchSeries) / times
	n := 0
	for i := 0; i < times; i++ {
		var end int
		// In many cases the batchSize is not perfect
		// so the last one will be a bit bigger or smaller than the others
		if i < times-1 {
			end = n + batchSize
		} else {
			end = len(sl.SketchSeries)
		}
		newSL := sl.SketchSeries[n:end]
		splitPayloads[i] = SketchSeriesList{
			buf:          sl.buf,
			SketchSeries: newSL,
		}
		n += batchSize
	}
	return splitPayloads, nil
}
