package metrics

import (
	"bytes"
	"encoding/json"
	"expvar"

	"github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/config"
	protostream "github.com/DataDog/datadog-agent/pkg/proto/stream"
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
	var err error
	var compressor *stream.Compressor
	buf := bufferContext.PrecompressionBuf
	ps := protostream.NewProtoStream()
	ps.Reset(buf)
	payloads := []*[]byte{}

	// constants for the protobuf data we will be writing, taken from
	// https://github.com/DataDog/agent-payload/blob/a2cd634bc9c088865b75c6410335270e6d780416/proto/metrics/agent_payload.proto#L47-L81
	const payloadSketches = 1
	const payloadMetadata = 2
	const sketchMetric = 1
	const sketchHost = 2
	const sketchDistributions = 3
	const sketchTags = 4
	const sketchDogsketches = 7
	/* unused
	const distributionTs = 1
	const distributionCnt = 2
	const distributionMin = 3
	const distributionMax = 4
	const distributionAvg = 5
	const distributionSum = 6
	const distributionV = 7
	const distributionG = 8
	const distributionDelta = 9
	const distributionBuf = 10
	*/
	const dogsketchTs = 1
	const dogsketchCnt = 2
	const dogsketchMin = 3
	const dogsketchMax = 4
	const dogsketchAvg = 5
	const dogsketchSum = 6
	const dogsketchK = 7
	const dogsketchN = 8

	// generate a footer containing an empty Metadata field (TODO: this isn't
	// necessary; an omitted field will be assumed empty)
	var footer []byte
	{
		buf := bytes.NewBuffer([]byte{})
		ps := protostream.NewProtoStream()
		ps.Reset(buf)
		ps.Embedded(payloadMetadata, func(ps *protostream.ProtoStream) error {
			return nil
		})
		footer = buf.Bytes()
	}

	// Prepare to write the next payload
	startPayload := func() error {
		var err error

		if compressor != nil {
		}

		bufferContext.CompressorInput.Reset()
		bufferContext.CompressorOutput.Reset()

		compressor, err = stream.NewCompressor(bufferContext.CompressorInput, bufferContext.CompressorOutput, []byte{}, footer, []byte{})
		if err != nil {
			return err
		}

		return nil
	}

	finishPayload := func() error {
		var payload []byte
		// Since the compression buffer is full - flush it and rotate
		payload, err = compressor.Close()
		if err != nil {
			return err
		}

		payloads = append(payloads, &payload)

		return nil
	}

	// start things off
	err = startPayload()
	if err != nil {
		return nil, err
	}

	for _, ss := range sl {
		buf.Reset()
		err = ps.Embedded(payloadSketches, func(ps *protostream.ProtoStream) error {
			var err error

			err = ps.String(sketchMetric, ss.Name)
			if err != nil {
				return err
			}

			err = ps.String(sketchHost, ss.Host)
			if err != nil {
				return err
			}

			for _, tag := range ss.Tags {
				err = ps.String(sketchTags, tag)
				if err != nil {
					return err
				}
			}

			for _, p := range ss.Points {
				err = ps.Embedded(sketchDogsketches, func(ps *protostream.ProtoStream) error {
					b := p.Sketch.Basic
					k, n := p.Sketch.Cols()

					err = ps.Int64(dogsketchTs, p.Ts)
					if err != nil {
						return err
					}

					err = ps.Int64(dogsketchCnt, b.Cnt)
					if err != nil {
						return err
					}

					err = ps.Double(dogsketchMin, b.Min)
					if err != nil {
						return err
					}

					err = ps.Double(dogsketchMax, b.Max)
					if err != nil {
						return err
					}

					err = ps.Double(dogsketchAvg, b.Avg)
					if err != nil {
						return err
					}

					err = ps.Double(dogsketchSum, b.Sum)
					if err != nil {
						return err
					}

					err = ps.Sint32Packed(dogsketchK, k)
					if err != nil {
						return err
					}

					err = ps.Uint32Packed(dogsketchN, n)
					if err != nil {
						return err
					}

					return nil
				})
				if err != nil {
					return err
				}
			}

			return nil
		})
		if err != nil {
			return nil, err
		}

		// Compress the protobuf metadata and the marshaled sketch
		err = compressor.AddItem(buf.Bytes())
		switch err {
		case stream.ErrPayloadFull:
			expvarsPayloadFull.Add(1)
			tlmPayloadFull.Inc()

			err = finishPayload()
			if err != nil {
				return nil, err
			}

			err = startPayload()
			if err != nil {
				return nil, err
			}

			// Add it to the new compression buffer
			err = compressor.AddItem(buf.Bytes())
			if err == stream.ErrItemTooBig {
				// Item was too big, drop it
				expvarsItemTooBig.Add(1)
				tlmItemTooBig.Inc()
				continue
			}
			if err != nil {
				// Unexpected error bail out
				expvarsUnexpectedItemDrops.Add(1)
				tlmUnexpectedItemDrops.Inc()
				return nil, err
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
			return nil, err
		}
	}

	err = finishPayload()
	if err != nil {
		return nil, err
	}

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
