// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reader implements an iterator over encoded dogstatsd http payload.
package reader

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"

	agentpayload "github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/dogstatsdhttp"
)

type resource = metrics.Resource
type originInfo = agentpayload.Origin

// MetricDataRader is an iterator over data contained in MetricData part of the payload.
//
// Usage:
//
//	r := NewMetricDataRader(payload.MetricData)
//	if err := r.Initialize(); err != nil {
//	    return err
//	}
//	for r.HaveMoreMetrics() {
//	    if err := r.NextMetric(); err != nil {
//	        return err
//
//	    // Accessors for metric entry data can be called.
//	    for r.HaveMorePoints() {
//	        if err := r.NextPoint(); err != nil {
//	            return err
//	        }
//	        // Acessors for metric data point can be called.
//	    }
//	 }
type MetricDataReader struct {
	data *pb.MetricData

	// Indexes point the next unconsumed element
	metricIdx int
	pointIdx  int

	valsSint64Idx    int
	valsFloat32Idx   int
	valsFloat64Idx   int
	sketchNumBinsIdx int
	sketchBinsIdx    int

	pointsRemaining int

	// Accumulators for delta encoded columns
	nameRef           int64
	tagsRef           int64
	resourcesRef      int64
	sourceTypeNameRef int64
	originInfoRef     int64
	timestamp         int64

	// Dicts are pre-loaded with empty element at index zero
	dictNameStr        []string
	dictTagsStr        []string
	dictTagsets        [][]string
	dictResourceStr    []string
	dictResources      [][]*resource
	dictSourceTypeName []string
	dictOriginInfo     []*originInfo
}

// NewMetricDataReader creates new reader from data.
func NewMetricDataReader(data *pb.MetricData) *MetricDataReader {
	return &MetricDataReader{
		data: data,
	}
}

// UnpackDicts reads and normalizes payload dictionaries for fast access.
func (r *MetricDataReader) Initialize() error {
	var err error
	r.dictNameStr, err = unpackStrDict(r.data.DictNameStr)
	if err != nil {
		return err
	}
	r.dictTagsStr, err = unpackStrDict(r.data.DictTagsStr)
	if err != nil {
		return err
	}
	r.dictTagsets, err = r.unpackTagsetsDict()
	if err != nil {
		return err
	}
	r.dictResourceStr, err = unpackStrDict(r.data.DictResourceStr)
	if err != nil {
		return err
	}
	r.dictResources, err = r.unpackResourcesDict()
	if err != nil {
		return err
	}
	r.dictSourceTypeName, err = unpackStrDict(r.data.DictSourceTypeName)
	if err != nil {
		return err
	}
	r.dictOriginInfo, err = unpackOriginInfoDict(r.data.DictOriginInfo)
	if err != nil {
		return err
	}
	return nil
}

var (
	errUnexpectedEOF = errors.New("unexpected end of column")
	errOverflow      = errors.New("length field overflow")
	errBadReference  = errors.New("invalid reference")
)

func unpackStrDict(raw []byte) ([]string, error) {
	dict := []string{""}

	for len(raw) > 0 {
		length, n := binary.Uvarint(raw)
		if n == 0 {
			return nil, errUnexpectedEOF
		}
		if n < 0 {
			return nil, errOverflow
		}
		if length > uint64(math.MaxInt-n) {
			return nil, errOverflow
		}
		end := n + int(length)
		if end > len(raw) {
			return nil, errUnexpectedEOF
		}
		str := string(raw[n:end])
		dict = append(dict, str)
		raw = raw[end:]
	}
	return dict, nil
}

func (r *MetricDataReader) unpackTagsetsDict() ([][]string, error) {
	packed := r.data.DictTagsets
	tagsets := [][]string{nil}

	for len(packed) > 0 {
		size := packed[0]
		packed = packed[1:]
		if size < 0 || size > int64(len(packed)) {
			return nil, errUnexpectedEOF
		}
		tags := make([]string, 0, size)

		idx := int64(0)
		for i := int64(0); i < size; i++ {
			idx += packed[i]

			if idx < 0 {
				if -idx >= int64(len(tagsets)) {
					return nil, errBadReference
				}
				tags = append(tags, tagsets[-idx]...)
			} else {
				if idx >= int64(len(r.dictTagsStr)) {
					return nil, errBadReference
				}
				tags = append(tags, r.dictTagsStr[idx])
			}
		}
		packed = packed[size:]
		tagsets = append(tagsets, tags)
	}
	return tagsets, nil
}

func (r *MetricDataReader) unpackResourcesDict() ([][]*resource, error) {
	packedLen := r.data.DictResourceLen
	packedType := r.data.DictResourceType
	packedName := r.data.DictResourceName
	resourcesDict := make([][]*resource, 1, len(packedLen)+1)

	start := int64(0)
	for _, size := range packedLen {
		if size < 0 {
			return nil, errUnexpectedEOF
		}
		end := start + size
		if end > int64(len(packedType)) || end > int64(len(packedName)) {
			return nil, errBadReference
		}

		typeRef := int64(0)
		nameRef := int64(0)
		resourcesSet := make([]*resource, size)
		resourcesDict = append(resourcesDict, resourcesSet)
		for i := int64(0); i < size; i++ {
			typeRef += packedType[start+i]
			nameRef += packedName[start+i]

			if typeRef < 0 || typeRef >= int64(len(r.dictResourceStr)) ||
				nameRef < 0 || nameRef >= int64(len(r.dictResourceStr)) {
				return nil, errBadReference
			}

			resourcesSet[i] = &resource{
				Type: r.dictResourceStr[typeRef],
				Name: r.dictResourceStr[nameRef],
			}
		}
		start = end
	}

	return resourcesDict, nil
}

func unpackOriginInfoDict(raw []int32) ([]*originInfo, error) {
	nelem := len(raw) / 3
	if len(raw) != nelem*3 {
		return nil, errUnexpectedEOF
	}
	dict := make([]*originInfo, 1, nelem+1)
	for i := 0; i < len(raw); i += 3 {
		dict = append(dict, &originInfo{
			OriginProduct:  uint32(raw[i+0]),
			OriginCategory: uint32(raw[i+1]),
			OriginService:  uint32(raw[i+2]),
		})
	}

	return dict, nil
}

// HaveMoreMetrics returns true if there are more metrics to read.
func (r *MetricDataReader) HaveMoreMetrics() bool {
	return r.metricIdx < len(r.data.Types)
}

// NextMetric consumes next metric entry and prepares data for access.
//
// If this method returns an error the reader is in an invalid state and calling data access methods may panic.
func (r *MetricDataReader) NextMetric() error {
	if !r.HaveMoreMetrics() {
		return errUnexpectedEOF
	}

	if r.metricIdx >= 0 {
		for r.HaveMorePoints() {
			if err := r.NextPoint(); err != nil {
				return err
			}
		}
	}

	r.metricIdx++

	if r.metricIdx > len(r.data.Types) ||
		r.metricIdx > len(r.data.Names) ||
		r.metricIdx > len(r.data.Tags) ||
		r.metricIdx > len(r.data.Resources) ||
		r.metricIdx > len(r.data.SourceTypeNames) ||
		r.metricIdx > len(r.data.OriginInfos) ||
		r.metricIdx > len(r.data.Intervals) ||
		r.metricIdx > len(r.data.NumPoints) {
		return errUnexpectedEOF
	}

	r.pointsRemaining = int(r.NumPoints())

	r.nameRef += r.data.Names[r.metricIdx-1]
	if r.nameRef < 0 || r.nameRef >= int64(len(r.dictNameStr)) {
		return errBadReference
	}

	r.tagsRef += r.data.Tags[r.metricIdx-1]
	if r.tagsRef < 0 || r.tagsRef >= int64(len(r.dictTagsets)) {
		return errBadReference
	}

	r.resourcesRef += r.data.Resources[r.metricIdx-1]
	if r.resourcesRef < 0 || r.resourcesRef >= int64(len(r.dictResources)) {
		return errBadReference
	}

	r.sourceTypeNameRef += r.data.SourceTypeNames[r.metricIdx-1]
	if r.sourceTypeNameRef < 0 || r.sourceTypeNameRef >= int64(len(r.dictSourceTypeName)) {
		return errBadReference
	}

	r.originInfoRef += r.data.OriginInfos[r.metricIdx-1]
	if r.originInfoRef < 0 || r.originInfoRef >= int64(len(r.dictOriginInfo)) {
		return errBadReference
	}

	return nil
}

func (r *MetricDataReader) packedType() uint64 {
	return r.data.Types[r.metricIdx-1]
}

// Type returns type of current metric entry.
func (r *MetricDataReader) Type() pb.MetricType {
	return pb.MetricType(r.packedType() & 0xF)
}

func (r *MetricDataReader) ValueType() pb.ValueType {
	return pb.ValueType(r.packedType() & 0xF0)
}

// Name returns metric name of current metric entry.
func (r *MetricDataReader) Name() string {
	return r.dictNameStr[r.nameRef]
}

// Tags returns set of tags for current metric entry.
func (r *MetricDataReader) Tags() []string {
	return r.dictTagsets[r.tagsRef]
}

// Resources returns set of resources for current metric entry.
func (r *MetricDataReader) Resources() []*resource {
	return r.dictResources[r.resourcesRef]
}

// SourceTypeName returns source type identifier for current metric entry.
func (r *MetricDataReader) SourceTypeName() string {
	return r.dictSourceTypeName[r.sourceTypeNameRef]
}

// Origin returns product origin information for current metric entry.
func (r *MetricDataReader) Origin() *originInfo {
	return r.dictOriginInfo[r.originInfoRef]
}

// Interval returns metric time interval for current metric entry.
func (r *MetricDataReader) Interval() uint64 {
	return r.data.Intervals[r.metricIdx-1]
}

// NumPoints returns number of data points contained in the current metric entry.
func (r *MetricDataReader) NumPoints() uint64 {
	return r.data.NumPoints[r.metricIdx-1]
}

// HaveMorePoints returns true if there are more points to read.
func (r *MetricDataReader) HaveMorePoints() bool {
	return r.pointsRemaining > 0
}

// NextPoint consumes next urnead metric data point and prepares data for access.
//
// If this method returns an error the reader is in an invalid state and calling data access methods may panic.
func (r *MetricDataReader) NextPoint() error {
	if !r.HaveMorePoints() {
		return errUnexpectedEOF
	}

	r.pointIdx++
	r.pointsRemaining--

	if r.pointIdx > len(r.data.Timestamps) {
		return errUnexpectedEOF
	}

	switch r.Type() {
	case pb.MetricType_Sketch:
		r.Debug()
		r.sketchNumBinsIdx++
		r.sketchBinsIdx += r.SketchNumBins()
		switch r.ValueType() {
		case pb.ValueType_Float64:
			r.valsFloat64Idx += 3
			r.valsSint64Idx++
		case pb.ValueType_Float32:
			r.valsFloat32Idx += 3
			r.valsSint64Idx++
		case pb.ValueType_Sint64:
			r.valsSint64Idx += 4
		case pb.ValueType_Zero:
			r.valsSint64Idx++
		}
	default:
		switch r.ValueType() {
		case pb.ValueType_Float64:
			r.valsFloat64Idx++
		case pb.ValueType_Float32:
			r.valsFloat32Idx++
		case pb.ValueType_Sint64:
			r.valsSint64Idx++
		}
	}

	if r.valsFloat64Idx > len(r.data.ValsFloat64) {
		return errUnexpectedEOF
	}
	if r.valsFloat32Idx > len(r.data.ValsFloat32) {
		return errUnexpectedEOF
	}
	if r.valsSint64Idx > len(r.data.ValsSint64) {
		return errUnexpectedEOF
	}
	if r.sketchNumBinsIdx > len(r.data.SketchNumBins) {
		return errUnexpectedEOF
	}
	if r.sketchBinsIdx > len(r.data.SketchBinKeys) {
		return errUnexpectedEOF
	}
	if r.sketchBinsIdx > len(r.data.SketchBinCnts) {
		return errUnexpectedEOF
	}

	r.timestamp += r.data.Timestamps[r.pointIdx-1]

	return nil
}

// Timestamp returns timestamp for current metric data point.
func (r *MetricDataReader) Timestamp() int64 {
	return r.timestamp
}

// Value returns metric value for current metric data point.
//
// Only valid to call if r.Type() != MetricType_Sketch, panics otherwise.
func (r *MetricDataReader) Value() float64 {
	if r.Type() == pb.MetricType_Sketch {
		panic("invalid type")
	}
	switch r.ValueType() {
	case pb.ValueType_Float64:
		return r.data.ValsFloat64[r.valsFloat64Idx-1]
	case pb.ValueType_Float32:
		return float64(r.data.ValsFloat32[r.valsFloat32Idx-1])
	case pb.ValueType_Sint64:
		return float64(r.data.ValsSint64[r.valsSint64Idx-1])
	default:
		return 0
	}
}

// SketchSummary returns sketch summary for current metric data point.
//
// Only valid if r.Type() == MetricType_Sketch, panics otherwise.
func (r *MetricDataReader) SketchSummary() (sum, min, max float64, cnt uint64) {
	if r.Type() != pb.MetricType_Sketch {
		panic("invalid type")
	}

	cnt = uint64(r.data.ValsSint64[r.valsSint64Idx-1])

	switch r.ValueType() {
	case pb.ValueType_Zero:
	case pb.ValueType_Sint64:
		sum = float64(r.data.ValsSint64[r.valsSint64Idx-4])
		min = float64(r.data.ValsSint64[r.valsSint64Idx-3])
		max = float64(r.data.ValsSint64[r.valsSint64Idx-2])
		// -1 is cnt
	case pb.ValueType_Float32:
		sum = float64(r.data.ValsFloat32[r.valsFloat32Idx-3])
		min = float64(r.data.ValsFloat32[r.valsFloat32Idx-2])
		max = float64(r.data.ValsFloat32[r.valsFloat32Idx-1])
	case pb.ValueType_Float64:
		sum = r.data.ValsFloat64[r.valsFloat64Idx-3]
		min = r.data.ValsFloat64[r.valsFloat64Idx-2]
		max = r.data.ValsFloat64[r.valsFloat64Idx-1]
	}

	return
}

// SketchNumBins returns number of sketch bins for the current metric data point.
//
// Only valid if r.Type() == MetricType_Sketch, panics otherwise.
func (r *MetricDataReader) SketchNumBins() int {
	if r.Type() != pb.MetricType_Sketch {
		panic("invalid type")
	}
	return int(r.data.SketchNumBins[r.sketchNumBinsIdx-1])
}

// SketchCols returns sketch data columns for the current metric data
// point.
//
// Only valid if r.Type() == MetricType_Sketch, panics otherwise.
func (r *MetricDataReader) SketchCols() (k []int32, n []uint32) {
	if r.Type() != pb.MetricType_Sketch {
		panic("invalid type")
	}
	size := r.SketchNumBins()
	start := r.sketchBinsIdx - size
	k = slices.Clone(r.data.SketchBinKeys[start:][:size])
	n = slices.Clone(r.data.SketchBinCnts[start:][:size])
	deltaDecode(k)
	return
}

func deltaDecode(s []int32) {
	for i := 1; i < len(s); i++ {
		s[i] += s[i-1]
	}
}

func (r *MetricDataReader) Debug() {
	fmt.Printf(`--
	metricIdx      %d
	pointIdx       %d
	valsSint64Idx  %d
	valsFloat32Idx %d
	valsFloat64Idx %d

	sketchNumBinsIdx %d
	sketchBinsIdx    %d

	pointsRemaining %d

	nameRef           %d
	tagsRef           %d
	resourcesRef      %d
	sourceTypeNameRef %d
	originInfoRef     %d
	timestamp         %d
`,
		// Indexes point to the last consumed element
		r.metricIdx,
		r.pointIdx,
		r.valsSint64Idx,
		r.valsFloat32Idx,
		r.valsFloat64Idx,
		r.sketchNumBinsIdx,
		r.sketchBinsIdx,

		r.pointsRemaining,

		// Accumulators for delta encoded columns
		r.nameRef,
		r.tagsRef,
		r.resourcesRef,
		r.sourceTypeNameRef,
		r.originInfoRef,
		r.timestamp,
	)

}
