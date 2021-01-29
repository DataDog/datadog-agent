package metrics

import (
	"bytes"
	"encoding/json"

	"github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/stream"
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

// A SketchSeriesList implements marshaler.Marshaler
type SketchSeriesList []SketchSeries

// MarshalJSON serializes sketch series to JSON.
// Quite slow, but hopefully this method is called only in the `agent check` command
func (sl SketchSeriesList) MarshalJSON() ([]byte, error) {
	// We use this function to customize generated JSON
	// This function, only used when displaying `bins`, is especially slow
	// As `StructToMap` function is using reflection to return a generic map[string]interface{}
	customSketchSeries := func(srcSl SketchSeriesList) []interface{} {
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
			"sketches": customSketchSeries(sl),
		}

		reqBody := &bytes.Buffer{}
		err := json.NewEncoder(reqBody).Encode(data)
		return reqBody.Bytes(), err
	}

	type SketchSeriesAlias SketchSeriesList
	data := map[string]SketchSeriesAlias{
		"sketches": SketchSeriesAlias(sl),
	}

	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// MarshalSplitCompress uses the stream compressor to marshal and compress sketch series in constant space.
// If a payload is larger than the max, a new payload will be generated.
func (sl SketchSeriesList) MarshalSplitCompress() ([]*[]byte, error) {
	// func (sl SketchSeriesList) MarshalSplitCompress() (forwarder.Payloads, error) {
	input := bytes.NewBuffer(make([]byte, 0, 1024))
	output := bytes.NewBuffer(make([]byte, 0, 1024))

	// The Metadata field of SketchPayload is never written to - so pack an empty metadata as the footer
	footer := []byte{0x12, 0}

	compressor, e := stream.NewCompressor(input, output, []byte{}, footer, []byte{})
	if e != nil {
		return nil, e
	}
	payloads := []*[]byte{}
	precompressionBuf := make([]byte, 1024)

	dsl := make([]gogen.SketchPayload_Sketch_Dogsketch, 1)
	for _, ss := range sl {
		if len(ss.Points) > cap(dsl) {
			dsl = append(dsl, make([]gogen.SketchPayload_Sketch_Dogsketch, len(ss.Points)-cap(dsl))...)
			dsl = dsl[:cap(dsl)]
		}

		for i, p := range ss.Points {
			b := p.Sketch.Basic
			k, n := p.Sketch.Cols()
			dsl[i] = gogen.SketchPayload_Sketch_Dogsketch{
				Ts:  p.Ts,
				Cnt: b.Cnt,
				Min: b.Min,
				Max: b.Max,
				Avg: b.Avg,
				Sum: b.Sum,
				K:   k,
				N:   n,
			}
		}

		sketch := gogen.SketchPayload_Sketch{
			Metric:      ss.Name,
			Host:        ss.Host,
			Tags:        ss.Tags,
			Dogsketches: dsl[:len(ss.Points)],
		}

		// Pack the protobuf metadata
		i := 0
		precompressionBuf[i] = 0xa
		i++
		i = encodeVarintAgentPayload(precompressionBuf, i, uint64(sketch.Size()))

		// Resize the pre-compression buffer if needed
		totalItemSize := sketch.Size() + i
		if totalItemSize > cap(precompressionBuf) {
			precompressionBuf = append(precompressionBuf, make([]byte, totalItemSize-cap(precompressionBuf))...)
			precompressionBuf = precompressionBuf[:cap(precompressionBuf)]
		}

		// Marshal the sketch to the precompression buffer
		_, e := sketch.MarshalTo(precompressionBuf[i:])
		if e != nil {
			return nil, e
		}

		// Compress the protobuf metadata and the marshaled sketch
		switch compressor.AddItem(precompressionBuf[:totalItemSize]) {
		case stream.ErrItemTooBig, stream.ErrPayloadFull:
			// Since the compression buffer is full - flush it and rotate
			payload, e := compressor.Close()
			if e != nil {
				return nil, e
			}
			payloads = append(payloads, &payload)
			input.Reset()
			output.Reset()
			compressor, e = stream.NewCompressor(input, output, []byte{}, footer, []byte{})
			if e != nil {
				return nil, e
			}

			// Add it to the new compression buffer - since this is a new compressor it should never overflow.
			e = compressor.AddItem(precompressionBuf[:totalItemSize])
			if e != nil {
				return nil, e
			}
		}
	}

	payload, e := compressor.Close()
	if e != nil {
		return nil, e
	}
	payloads = append(payloads, &payload)

	// return payloads, nonCompressed
	return payloads, nil
}

// taken from agent_payload.pb.go
func encodeVarintAgentPayload(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
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
