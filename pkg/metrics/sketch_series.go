package metrics

import (
	"bytes"
	"encoding/json"
	"expvar"

	"github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/stream"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
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

var (
	expvars                    = expvar.NewMap("sketch_series")
	expvarsItemTooBig          = expvar.Int{}
	expvarsPayloadFull         = expvar.Int{}
	expvarsUnexpectedItemDrops = expvar.Int{}
	tlmItemTooBig              = telemetry.NewCounter("sketch_series", "sketch_too_big",
		nil, "Number of payloads dropped because they were too big for the stream compressor")
	tlmPayloadFull = telemetry.NewCounter("sketch_series", "payload_full",
		nil, "How many times we've hit a 'payload is full' in the stream compressor")
	tlmUnexpectedItemDrops = telemetry.NewCounter("sketch_series", "unexpected_item_drops",
		nil, "Items dropped in the stream compressor")
)

func init() {
	expvars.Set("ItemTooBig", &expvarsItemTooBig)
	expvars.Set("PayloadFull", &expvarsPayloadFull)
	expvars.Set("UnexpectedItemDrops", &expvarsUnexpectedItemDrops)
}

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

// MarshalSplitCompress uses the stream compressor to marshal and compress sketch series payloads.
// If a compressed payload is larger than the max, a new payload will be generated. This method returns a slice of
// compressed protobuf marshaled gogen.SketchPayload objects. gogen.SketchPayload is not directly marshaled - instead
// it's contents are marshaled individually, packed with the appropriate protobuf metadata, and compressed in stream.
// The resulting payloads (when decompressed) are binary equal to the result of marshaling the whole object at once.
func (sl SketchSeriesList) MarshalSplitCompress(bufferContext *marshaler.BufferContext) ([]*[]byte, error) {
	// The Metadata field of gogen.SketchPayload is never written to - so pack an empty metadata as the footer
	footer := []byte{0x12, 0}

	bufferContext.CompressorInput.Reset()
	bufferContext.CompressorOutput.Reset()

	compressor, e := stream.NewCompressor(bufferContext.CompressorInput, bufferContext.CompressorOutput, []byte{}, footer, []byte{})
	if e != nil {
		return nil, e
	}
	payloads := []*[]byte{}

	dsl := make([]gogen.SketchPayload_Sketch_Dogsketch, 1)
	for _, ss := range sl {
		if len(ss.Points) > cap(dsl) {
			dsl = make([]gogen.SketchPayload_Sketch_Dogsketch, len(ss.Points))
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

		// Pack the protobuf metadata - see SketchPayload.MarshalTo in agent_payload.pb.go for reference.
		metadataSize := 0
		// Magic number that occurs before the varint encoding
		bufferContext.PrecompressionBuf[metadataSize] = 0xa
		metadataSize++
		metadataSize = encodeVarintAgentPayload(bufferContext.PrecompressionBuf, metadataSize, uint64(sketch.Size()))

		// Resize the pre-compression buffer if needed
		totalItemSize := sketch.Size() + metadataSize
		if totalItemSize > cap(bufferContext.PrecompressionBuf) {
			bufferContext.PrecompressionBuf = append(bufferContext.PrecompressionBuf, make([]byte, totalItemSize-cap(bufferContext.PrecompressionBuf))...)
			bufferContext.PrecompressionBuf = bufferContext.PrecompressionBuf[:cap(bufferContext.PrecompressionBuf)]
		}

		// Marshal the sketch to the precompression buffer after the metadata
		_, e := sketch.MarshalTo(bufferContext.PrecompressionBuf[metadataSize:])
		if e != nil {
			return nil, e
		}

		// Compress the protobuf metadata and the marshaled sketch
		switch compressor.AddItem(bufferContext.PrecompressionBuf[:totalItemSize]) {
		case stream.ErrPayloadFull:
			expvarsPayloadFull.Add(1)
			tlmPayloadFull.Inc()

			// Since the compression buffer is full - flush it and rotate
			payload, e := compressor.Close()
			if e != nil {
				return nil, e
			}
			payloads = append(payloads, &payload)
			bufferContext.CompressorInput.Reset()
			bufferContext.CompressorOutput.Reset()
			compressor, e = stream.NewCompressor(bufferContext.CompressorInput, bufferContext.CompressorOutput, []byte{}, footer, []byte{})
			if e != nil {
				return nil, e
			}

			// Add it to the new compression buffer
			e = compressor.AddItem(bufferContext.PrecompressionBuf[:totalItemSize])
			if e == stream.ErrItemTooBig {
				// Item was too big, drop it
				expvarsItemTooBig.Add(1)
				tlmItemTooBig.Inc()
				continue
			}
			if e != nil {
				// Unexpected error bail out
				expvarsUnexpectedItemDrops.Add(1)
				tlmUnexpectedItemDrops.Inc()
				return nil, e
			}
		case stream.ErrItemTooBig:
			// Item was too big, drop it
			expvarsItemTooBig.Add(1)
			tlmItemTooBig.Add(1)
		case nil:
			continue
		default:
			// Unexpected error bail out
			expvarsUnexpectedItemDrops.Add(1)
			tlmUnexpectedItemDrops.Inc()
			return nil, e
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
