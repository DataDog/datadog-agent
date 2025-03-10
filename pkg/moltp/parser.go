// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package moltp

import (
	"errors"
	"math"

	common "go.opentelemetry.io/proto/otlp/common/v1"
	resource "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/quantile"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
	"github.com/golang/protobuf/proto"
	"github.com/richardartoul/molecule"
	"github.com/richardartoul/molecule/src/codec"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

type bucket struct {
	offset int32
	counts []uint64
}

func (b *bucket) reset() {
	b.offset = 0
	b.counts = b.counts[0:0]
}

func (b *bucket) toStore(s *store.DenseStore) {
	for idx, cnt := range b.counts {
		if cnt > 0 {
			s.AddWithCount(int(b.offset)+idx, float64(cnt))
		}
	}
}

type ctx struct {
	resource resource.Resource
	scope    common.InstrumentationScope
	name     string
	time     uint64

	attrKey []byte
	attrVal []byte
	attrBuf []byte
	attrs   []string

	histCnt   uint64
	histSum   float64
	histScale int32
	histPos   bucket
	histNeg   bucket
	histMin   float64
	histMax   float64
	histZero  uint64

	storePos *store.DenseStore
	storeNeg *store.DenseStore

	serieSink    metrics.SerieSink
	sketchesSink metrics.SketchesSink

	histConvFailed uint
}

func newCtx(serieSink metrics.SerieSink, sketchesSink metrics.SketchesSink) *ctx {
	return &ctx{
		storePos: store.NewDenseStore(),
		storeNeg: store.NewDenseStore(),

		serieSink:    serieSink,
		sketchesSink: sketchesSink,
	}
}

func parseResource(cx *ctx, val molecule.Value) error {
	var res resource.Resource
	if err := proto.Unmarshal(val.Bytes, &res); err != nil {
		return err
	}
	proto.Merge(&cx.resource, &res)
	return nil
}
func resetResource(cx *ctx, _val molecule.Value) error {
	cx.resource = resource.Resource{}
	return nil
}
func parseScope(cx *ctx, val molecule.Value) error {
	var scp common.InstrumentationScope
	if err := proto.Unmarshal(val.Bytes, &scp); err != nil {
		return err
	}
	proto.Merge(&cx.scope, &scp)
	return nil
}
func resetScope(cx *ctx, _val molecule.Value) error {
	cx.scope = common.InstrumentationScope{}
	return nil
}
func parseName(cx *ctx, val molecule.Value) error {
	cx.name = string(val.Bytes)
	return nil
}
func parseTime(cx *ctx, val molecule.Value) error {
	cx.time = val.Number
	return nil
}
func parseAttrKey(cx *ctx, val molecule.Value) error {
	cx.attrKey = val.Bytes
	return nil
}
func parseAttrStr(cx *ctx, val molecule.Value) error {
	cx.attrVal = val.Bytes
	return nil
}
func appendAttr(cx *ctx, _val molecule.Value) error {
	cx.attrBuf = cx.attrBuf[0:0]
	cx.attrBuf = append(cx.attrBuf, cx.attrKey...)
	cx.attrBuf = append(cx.attrBuf, byte(':'))
	cx.attrBuf = append(cx.attrBuf, cx.attrVal...)
	cx.attrs = append(cx.attrs, string(cx.attrBuf))
	cx.attrKey = nil
	cx.attrVal = nil
	return nil
}

func flushHistogramPoint(cx *ctx, _val molecule.Value) error {
	defer func() {
		cx.attrs = nil
		cx.histCnt = 0
		cx.histSum = 0
		cx.histScale = 0
		cx.histPos.reset()
		cx.histNeg.reset()
		cx.histMin = 0
		cx.histMax = 0
		cx.histZero = 0
		cx.storePos.Clear()
		cx.storeNeg.Clear()
	}()

	cx.histPos.toStore(cx.storePos)
	cx.histNeg.toStore(cx.storeNeg)
	gamma := math.Pow(2, math.Pow(2, float64(-cx.histScale)))
	mapping, err := mapping.NewLogarithmicMappingWithGamma(gamma, 0)
	if err != nil {
		cx.histConvFailed++
		return nil
	}
	sk := ddsketch.NewDDSketch(mapping, cx.storePos, cx.storeNeg)
	_ = sk.AddWithCount(0, float64(cx.histZero)) // cannot fail when the value is zero
	as, err := quantile.ConvertDDSketchIntoSketch(sk)
	if err != nil {
		cx.histConvFailed++
		return nil
	}

	if cx.histCnt > math.MaxInt64 {
		cx.histConvFailed++
		return nil
	}

	as.Basic.Cnt = int64(cx.histCnt)
	as.Basic.Sum = cx.histSum
	as.Basic.Avg = cx.histSum / float64(cx.histCnt)
	as.Basic.Min = cx.histMin
	as.Basic.Max = cx.histMax

	cx.sketchesSink.Append(&metrics.SketchSeries{
		Name: cx.name,
		Tags: tagset.CompositeTagsFromSlice(cx.attrs), // consume attrs
		// Host: TODO
		// Source: TODO
		Points: []metrics.SketchPoint{{
			Ts:     int64(cx.time / 1e9),
			Sketch: as,
		}}})

	return nil
}

func parseHistogramSum(cx *ctx, val molecule.Value) error {
	v, err := val.AsDouble()
	if err != nil {
		return err
	}
	cx.histSum = v
	return nil
}
func parseHistogramCnt(cx *ctx, val molecule.Value) error {
	cx.histCnt = val.Number
	return nil
}
func parseHistogramScale(cx *ctx, val molecule.Value) error {
	v, err := val.AsSint32()
	if err != nil {
		return err
	}
	cx.histScale = v
	return nil
}
func parseHistogramZero(cx *ctx, val molecule.Value) error {
	cx.histZero = val.Number
	return nil
}
func parseHistgoramBucketsPos(cx *ctx, val molecule.Value) error {
	return parseHistogramBuckets(&cx.histPos, val)
}
func parseHistogramBucketsNeg(cx *ctx, val molecule.Value) error {
	return parseHistogramBuckets(&cx.histNeg, val)
}
func parseHistogramMin(cx *ctx, val molecule.Value) error {
	v, err := val.AsDouble()
	if err != nil {
		return err
	}
	cx.histMin = v
	return nil
}
func parseHistogramMax(cx *ctx, val molecule.Value) error {
	v, err := val.AsDouble()
	if err != nil {
		return err
	}
	cx.histMax = v
	return nil
}

var errInvalidBucket = errors.New("invalid bucket format")

func parseHistogramBuckets(b *bucket, val molecule.Value) error {
	if val.WireType != codec.WireBytes {
		return errInvalidType
	}

	buf := codec.NewBuffer(val.Bytes)
	for !buf.EOF() {
		id, err := molecule.Next(buf, &val)
		if err != nil {
			return err
		}
		switch id {
		case 1:
			b.offset, err = val.AsInt32()
			if err != nil {
				return err
			}
		case 2:
			if val.WireType != codec.WireBytes {
				return errInvalidBucket
			}
			buf := codec.NewBuffer(val.Bytes)
			b.counts = b.counts[0:0]
			for !buf.EOF() {
				cnt, err := buf.DecodeVarint()
				if err != nil {
					return err
				}
				b.counts = append(b.counts, cnt)
			}
		}
	}
	return nil
}

// Protobuf fields can come in any order, repeated fields can be interleaved with any other. Non-repeated
// fields can repeat anyway and must be merged with earlier instances. See spec for details.
type fieldByNum []message // index = fieldNum - 1
type fields []fieldByNum  // we need to visit certain fields before others
type message struct {
	name    string // for debugging
	fields  fields
	handler func(cx *ctx, val molecule.Value) error
}

// requestFields corresponds to ExportMetricsServiceRequest
var exportMetricsRequest = message{
	name: "exportMetricsRequest",
	fields: fields{{
		// 1: repeeated ResourceMetrics
		{
			name:    "resourceMetrics",
			handler: resetResource,
			fields: fields{
				{
					// 1: Resource
					{name: "resource", handler: parseResource},
				}, {
					/* 1 */ {},
					// 2: repeated ScopeMetrics
					{
						name:    "scopeMetrics",
						handler: resetScope,
						fields: fields{
							{
								// 1: Scope
								{name: "scope", handler: parseScope},
							}, {
								/* 1 */ {},
								// 2: repeated Metric
								{
									fields: fields{
										{
											/* 1 */ {name: "name", handler: parseName},
										}, {
											/* 1 */ {},
											/* 2 */ {},
											/* 3 */ {},
											/* 4 */ {},
											/* 5 */ {},
											/* 6 */ {},
											/* 7 */ {},
											/* 8 */ {},
											/* 9 */ {},
											/* 10 */ exponentialHistogram,
										}}}}}}}}}}}}

var exponentialHistogram = message{
	name: "exponentialHistogram",
	fields: fields{{
		// 1: repeated ExponentialHistgoramDataPoint
		{
			name:    "exponentialHistogramDataPoint",
			handler: flushHistogramPoint,
			fields: fields{{
				/* 1: repeated KeyValue */ attributes,
				/* 2 */ {},
				/* 3 */ {name: "time", handler: parseTime},
				/* 4 */ {name: "histogramCnt", handler: parseHistogramCnt},
				/* 5 */ {name: "histogramSum", handler: parseHistogramSum},
				/* 6 */ {name: "histogramScale", handler: parseHistogramScale},
				/* 7 */ {name: "histgoramZero", handler: parseHistogramZero},
				/* 8 */ {name: "histogramBucketPos", handler: parseHistgoramBucketsPos},
				/* 9 */ {name: "histogramBucketNeg", handler: parseHistogramBucketsNeg},
				/* 10 */ {},
				/* 11 */ {},
				/* 12 */ {name: "histogramMin", handler: parseHistogramMin},
				/* 13 */ {name: "histogramMax", handler: parseHistogramMax},
			}}}}}}

// repeated KeyValue
var attributes = message{
	name:    "keyValue",
	handler: appendAttr,
	fields: fields{{
		/* 1 */ {name: "name", handler: parseAttrKey},
		// 2: AnyValue
		{name: "value", fields: fields{{
			/* 1 */ {name: "strVal", handler: parseAttrStr},
		}}}}}}

func (m *message) parseBytes(cx *ctx, data []byte) error {
	return m.parse(cx, molecule.Value{WireType: codec.WireBytes, Bytes: data})
}

var errInvalidType = errors.New("invalid wire type for message with fields")

func (m *message) parse(cx *ctx, inp molecule.Value) error {
	buf := codec.Buffer{}

	for _, fd := range m.fields {
		if inp.WireType != codec.WireBytes {
			return errInvalidType
		}
		buf.Reset(inp.Bytes)
		for !buf.EOF() {
			val := molecule.Value{}
			num, err := molecule.Next(&buf, &val)
			if err != nil {
				return err
			}
			if num > 0 && int(num-1) < len(fd) {
				if err := fd[num-1].parse(cx, val); err != nil {
					return err
				}
			}
		}
	}

	if handler := m.handler; handler != nil {
		if err := handler(cx, inp); err != nil {
			return err
		}
	}

	return nil
}
