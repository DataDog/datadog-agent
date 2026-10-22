// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"errors"
	"fmt"
	"math"
	"slices"

	intake_v3 "github.com/DataDog/agent-payload/v5/metrics/intake_v3"
)

// Point-level checks of the API v3 transport, as in the intake reader (dd-source
// domains/metrics/shared/libs/go/apiv3/histogram_reader.go).
var (
	errV3BadPointFlags    = errors.New("invalid histogram point flags")
	errV3BadBounds        = errors.New("explicit histogram bounds must be finite and strictly increasing")
	errV3BadZeroThreshold = errors.New("exponential histogram zero threshold must be finite and nonnegative")
	errV3BadBucketIndex   = errors.New("exponential histogram bucket index outside int32")
)

const v3KnownPointFlags = uint32(intake_v3.PointFlags_pointHasMin | intake_v3.PointFlags_pointHasMax | intake_v3.PointFlags_pointHasSum)

// ExplicitHistogramPoint is one point of an OTel explicit bucket histogram sent over API v3.
// Sum, Min and Max are nil when the agent did not send them.
type ExplicitHistogramPoint struct {
	Timestamp int64     `json:"timestamp"`
	Count     uint64    `json:"count"`
	Sum       *float64  `json:"sum,omitempty"`
	Min       *float64  `json:"min,omitempty"`
	Max       *float64  `json:"max,omitempty"`
	Bounds    []float64 `json:"bounds"`
	Counts    []uint64  `json:"counts"`
}

// ExponentialHistogramPoint is one point of an OTel exponential histogram sent over API v3.
// Sum, Min and Max are nil when the agent did not send them.
type ExponentialHistogramPoint struct {
	Timestamp      int64    `json:"timestamp"`
	Count          uint64   `json:"count"`
	Sum            *float64 `json:"sum,omitempty"`
	Min            *float64 `json:"min,omitempty"`
	Max            *float64 `json:"max,omitempty"`
	Scale          int32    `json:"scale"`
	ZeroCount      uint64   `json:"zero_count"`
	ZeroThreshold  float64  `json:"zero_threshold"`
	PositiveOffset int32    `json:"positive_offset"`
	Positive       []uint64 `json:"positive"`
	NegativeOffset int32    `json:"negative_offset"`
	Negative       []uint64 `json:"negative"`
}

// isHistogram reports whether the current metric entry is an OTel histogram.
func (r *metricReaderV3) isHistogram() bool {
	t := r.Type()
	return t == intake_v3.MetricType_ExplicitHistogram || t == intake_v3.MetricType_ExponentialHistogram
}

func (r *metricReaderV3) nextNumBins() (int, error) {
	md := r.payload.MetricData
	if r.sketchNumBinsIdx >= len(md.NumBins) {
		return 0, errV3UnexpectedEOF
	}
	n := md.NumBins[r.sketchNumBinsIdx]
	r.sketchNumBinsIdx++
	if n > uint64(len(md.BinCnts)-r.sketchBinCntsIdx) {
		return 0, errV3UnexpectedEOF
	}
	return int(n), nil
}

func (r *metricReaderV3) nextBinCnts(n int) []uint64 {
	counts := slices.Clone(r.payload.MetricData.BinCnts[r.sketchBinCntsIdx:][:n])
	r.sketchBinCntsIdx += n
	return counts
}

func (r *metricReaderV3) nextPointFlags() (uint32, error) {
	md := r.payload.MetricData
	if r.pointFlagsIdx >= len(md.PointFlags) {
		return 0, errV3UnexpectedEOF
	}
	flags := md.PointFlags[r.pointFlagsIdx]
	r.pointFlagsIdx++
	if flags&^v3KnownPointFlags != 0 {
		return 0, errV3BadPointFlags
	}
	return flags, nil
}

func (r *metricReaderV3) nextValUint64() (uint64, error) {
	md := r.payload.MetricData
	if r.valsUint64Idx >= len(md.ValsUint64) {
		return 0, errV3UnexpectedEOF
	}
	v := md.ValsUint64[r.valsUint64Idx]
	r.valsUint64Idx++
	return v, nil
}

func (r *metricReaderV3) nextSint64() (int64, error) {
	if r.valsSint64Idx >= len(r.payload.MetricData.ValsSint64) {
		return 0, errV3UnexpectedEOF
	}
	v := r.payload.MetricData.ValsSint64[r.valsSint64Idx]
	r.valsSint64Idx++
	return v, nil
}

func (r *metricReaderV3) nextInt32() (int32, error) {
	v, err := r.nextSint64()
	if err != nil {
		return 0, err
	}
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, errV3Overflow
	}
	return int32(v), nil
}

// nextValue reads one value from the column selected by the value type.
// The Zero value type takes no slot.
func (r *metricReaderV3) nextValue() (float64, error) {
	md := r.payload.MetricData
	switch r.ValueType() {
	case intake_v3.ValueType_Zero:
		return 0, nil
	case intake_v3.ValueType_Sint64:
		v, err := r.nextSint64()
		return float64(v), err
	case intake_v3.ValueType_Float32:
		if r.valsFloat32Idx >= len(md.ValsFloat32) {
			return 0, errV3UnexpectedEOF
		}
		r.valsFloat32Idx++
		return float64(md.ValsFloat32[r.valsFloat32Idx-1]), nil
	case intake_v3.ValueType_Float64:
		if r.valsFloat64Idx >= len(md.ValsFloat64) {
			return 0, errV3UnexpectedEOF
		}
		r.valsFloat64Idx++
		return md.ValsFloat64[r.valsFloat64Idx-1], nil
	}
	return 0, fmt.Errorf("unknown value type %d", r.ValueType())
}

// nextSummary reads sum, min and max in that order, skipping the absent ones.
func (r *metricReaderV3) nextSummary(flags uint32) (sum, min, max *float64, err error) {
	next := func(bit intake_v3.PointFlags) (*float64, error) {
		if flags&uint32(bit) == 0 {
			return nil, nil
		}
		v, err := r.nextValue()
		return &v, err
	}
	if sum, err = next(intake_v3.PointFlags_pointHasSum); err != nil {
		return
	}
	if min, err = next(intake_v3.PointFlags_pointHasMin); err != nil {
		return
	}
	max, err = next(intake_v3.PointFlags_pointHasMax)
	return
}

// readExplicitHistogramPoint decodes an explicit bucket histogram point: numBins and
// binCnts hold the bucket counts, valsUint64 the count, and the value column the present
// summary values followed by the bounds.
func (r *metricReaderV3) readExplicitHistogramPoint() error {
	n, err := r.nextNumBins()
	if err != nil {
		return err
	}
	p := ExplicitHistogramPoint{Counts: r.nextBinCnts(n)}
	flags, err := r.nextPointFlags()
	if err != nil {
		return err
	}
	if p.Count, err = r.nextValUint64(); err != nil {
		return err
	}
	if p.Sum, p.Min, p.Max, err = r.nextSummary(flags); err != nil {
		return err
	}
	if n > 0 {
		p.Bounds = make([]float64, n-1)
		for i := range p.Bounds {
			if p.Bounds[i], err = r.nextValue(); err != nil {
				return err
			}
			if math.IsNaN(p.Bounds[i]) || math.IsInf(p.Bounds[i], 0) || i > 0 && p.Bounds[i] <= p.Bounds[i-1] {
				return errV3BadBounds
			}
		}
	}
	r.explicitPoint = p
	return nil
}

// readExponentialHistogramPoint decodes an exponential histogram point: two numBins
// entries and the dense positive then negative counts, the scale and offsets in
// valsSint64, count and zero count in valsUint64, and the value column the present
// summary values followed by the zero threshold.
func (r *metricReaderV3) readExponentialHistogramPoint() error {
	var p ExponentialHistogramPoint
	pos, err := r.nextNumBins()
	if err != nil {
		return err
	}
	p.Positive = r.nextBinCnts(pos)
	neg, err := r.nextNumBins()
	if err != nil {
		return err
	}
	p.Negative = r.nextBinCnts(neg)

	if p.Scale, err = r.nextInt32(); err != nil {
		return err
	}
	if p.PositiveOffset, err = r.nextInt32(); err != nil {
		return err
	}
	if p.NegativeOffset, err = r.nextInt32(); err != nil {
		return err
	}
	if lastBucketIndexOverflows(p.PositiveOffset, pos) || lastBucketIndexOverflows(p.NegativeOffset, neg) {
		return errV3BadBucketIndex
	}

	flags, err := r.nextPointFlags()
	if err != nil {
		return err
	}
	if p.Count, err = r.nextValUint64(); err != nil {
		return err
	}
	if p.ZeroCount, err = r.nextValUint64(); err != nil {
		return err
	}
	if p.Sum, p.Min, p.Max, err = r.nextSummary(flags); err != nil {
		return err
	}
	if p.ZeroThreshold, err = r.nextValue(); err != nil {
		return err
	}
	if math.IsNaN(p.ZeroThreshold) || math.IsInf(p.ZeroThreshold, 0) || p.ZeroThreshold < 0 {
		return errV3BadZeroThreshold
	}
	r.exponentialPoint = p
	return nil
}

// lastBucketIndexOverflows reports whether the index of the last bucket of a nonempty
// range, offset+length-1, does not fit int32.
func lastBucketIndexOverflows(offset int32, length int) bool {
	return length > 0 && int64(offset)+int64(length)-1 > math.MaxInt32
}

// ExplicitHistogram returns the current explicit bucket histogram point.
//
// Only valid for explicit bucket histograms, panics otherwise.
func (r *metricReaderV3) ExplicitHistogram() ExplicitHistogramPoint {
	if r.Type() != intake_v3.MetricType_ExplicitHistogram {
		panic("invalid type")
	}
	p := r.explicitPoint
	p.Timestamp = r.timestamp
	return p
}

// ExponentialHistogram returns the current exponential histogram point.
//
// Only valid for exponential histograms, panics otherwise.
func (r *metricReaderV3) ExponentialHistogram() ExponentialHistogramPoint {
	if r.Type() != intake_v3.MetricType_ExponentialHistogram {
		panic("invalid type")
	}
	p := r.exponentialPoint
	p.Timestamp = r.timestamp
	return p
}
