// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package aggregator provides payload parsers and iterators for the V3 metrics intake wire format.
package aggregator

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	intake_v3 "github.com/DataDog/agent-payload/v5/metrics/intake_v3"
)

// metricReaderV3 is an iterator over data contained in intake_v3.Payload.
type metricReaderV3 struct {
	payload *intake_v3.Payload

	// Indexes point the next unconsumed element
	metricIdx  int
	pointIdx   int
	unitRefIdx int

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
	unitRef           int64

	// Dicts are pre-loaded with empty element at index zero
	dictNameStr        []string
	dictTagsStr        []string
	dictUnitStr        []string
	dictTagsets        [][]string
	dictResourceStr    []string
	dictResources      [][]*v3resource
	dictSourceTypeName []string
	dictOriginInfo     []*v3originInfo
}

type v3resource = [2]string
type v3originInfo = [3]int32

// NewReader creates an iterator over the data contained in intake_v3.MetricData
// by wrapping it in a Payload
//
//nolint:revive
func NewReader(data *intake_v3.MetricData) *metricReaderV3 {
	return &metricReaderV3{
		payload: &intake_v3.Payload{
			MetricData: data,
		},
	}
}

// NewPayloadReader creates an iterator over the data contained in intake_v3.Payload
//
//nolint:revive
func NewPayloadReader(payload *intake_v3.Payload) *metricReaderV3 {
	return &metricReaderV3{
		payload: payload,
	}
}

// Initialize reads and normalizes payload dictionaries for fast access.
// This method must be called before any other method on the reader.
func (r *metricReaderV3) Initialize() error {
	if r.payload.MetricData == nil {
		return errors.New("metric data must not be nil")
	}

	var err error
	r.dictNameStr, err = unpackStrDictV3(r.payload.MetricData.DictNameStr, false)
	if err != nil {
		return err
	}
	r.dictTagsStr, err = unpackStrDictV3(r.payload.MetricData.DictTagStr, true)
	if err != nil {
		return err
	}
	r.dictUnitStr, err = unpackStrDictV3(r.payload.MetricData.DictUnitStr, false)
	if err != nil {
		return err
	}
	r.dictTagsets, err = r.unpackTagsetsDictV3()
	if err != nil {
		return err
	}
	r.dictResourceStr, err = unpackStrDictV3(r.payload.MetricData.DictResourceStr, false)
	if err != nil {
		return err
	}
	r.dictResources, err = r.unpackResourcesDictV3()
	if err != nil {
		return err
	}
	r.dictSourceTypeName, err = unpackStrDictV3(r.payload.MetricData.DictSourceTypeName, false)
	if err != nil {
		return err
	}
	r.dictOriginInfo, err = unpackOriginInfoDictV3(r.payload.MetricData.DictOriginInfo)
	if err != nil {
		return err
	}
	return nil
}

var (
	errV3UnexpectedEOF = errors.New("unexpected end of column")
	errV3Overflow      = errors.New("length field overflow")
	errV3BadReference  = errors.New("invalid reference")
	errV3InvalidUTF8   = errors.New("invalid UTF-8 string")
)

func unpackStrDictV3(raw []byte, sanitizeInvalidUTF8 bool) ([]string, error) {
	dict := []string{""}

	for len(raw) > 0 {
		length, n := binary.Uvarint(raw)
		if n == 0 {
			return nil, errV3UnexpectedEOF
		}
		if n < 0 {
			return nil, errV3Overflow
		}
		if length > uint64(math.MaxInt-n) {
			return nil, errV3Overflow
		}
		end := n + int(length)
		if end > len(raw) {
			return nil, errV3UnexpectedEOF
		}
		str := string(raw[n:end])

		if !utf8.ValidString(str) {
			if sanitizeInvalidUTF8 {
				str = strings.ToValidUTF8(str, string(utf8.RuneError))
			} else {
				return nil, errV3InvalidUTF8
			}
		}

		dict = append(dict, str)
		raw = raw[end:]
	}
	return dict, nil
}

func (r *metricReaderV3) unpackTagsetsDictV3() ([][]string, error) {
	packed := r.payload.MetricData.DictTagsets
	tagsets := [][]string{nil}

	metadataTags := r.payload.GetMetadata().GetTags()

	for len(packed) > 0 {
		size := packed[0]
		packed = packed[1:]
		if size < 0 || size > int64(len(packed)) {
			return nil, errV3UnexpectedEOF
		}
		tags := make([]string, 0, int(size)+len(metadataTags))

		idx := int64(0)
		for i := int64(0); i < size; i++ {
			idx += packed[i]

			if idx < 0 {
				if idx <= -math.MaxInt64 || -idx >= int64(len(tagsets)) {
					return nil, errV3BadReference
				}
				tags = append(tags, tagsets[-idx]...)
			} else {
				if idx >= int64(len(r.dictTagsStr)) {
					return nil, errV3BadReference
				}
				tags = append(tags, r.dictTagsStr[idx])
			}
		}
		packed = packed[size:]
		tagsets = append(tagsets, tags)
	}

	// Now do a one-time union of metric tags + metadata tags
	if len(metadataTags) == 0 {
		return tagsets, nil
	}

	metaIndex := make(map[string]int, len(metadataTags))
	for i, mt := range metadataTags {
		metaIndex[mt] = i
	}

	for i, tags := range tagsets {
		if len(tags) == 0 {
			// Just append all the metadata tags if the tagset is empty
			tagsets[i] = append(tags, metadataTags...)
			continue
		}

		// Track which metadata tags have already been seen
		seen := make([]bool, len(metadataTags))

		// Mark the metadata tags that have already been seen
		for _, t := range tags {
			if idx, ok := metaIndex[t]; ok {
				seen[idx] = true
			}
		}

		// Append only the metadata tags that are missing
		for idx, mt := range metadataTags {
			if !seen[idx] {
				tags = append(tags, mt)
			}
		}

		tagsets[i] = tags
	}

	return tagsets, nil
}

func (r *metricReaderV3) unpackResourcesDictV3() ([][]*v3resource, error) {
	packedLen := r.payload.MetricData.DictResourceLen
	packedType := r.payload.MetricData.DictResourceType
	packedName := r.payload.MetricData.DictResourceName
	resourcesDict := make([][]*v3resource, 1, len(packedLen)+1)

	metadataResources := r.payload.GetMetadata().GetResources()

	// Decode metadata resources once
	var metaResources []*v3resource
	if len(metadataResources) > 0 {
		if len(metadataResources)%2 != 0 {
			return nil, errors.New("metadata resources must be [Type, Name] pairs")
		}
		pairs := len(metadataResources) / 2
		metaResources = make([]*v3resource, pairs)
		for i := 0; i < pairs; i++ {
			t := metadataResources[2*i]
			n := metadataResources[2*i+1]
			metaResources[i] = &v3resource{t, n}
		}
	}

	start := int64(0)
	for _, size := range packedLen {
		if size < 0 {
			return nil, errV3UnexpectedEOF
		}
		if size > math.MaxInt64-start {
			return nil, errV3Overflow
		}
		end := start + size
		if end > int64(len(packedType)) || end > int64(len(packedName)) {
			return nil, errV3BadReference
		}

		typeRef := int64(0)
		nameRef := int64(0)
		resourcesSet := make([]*v3resource, 0, size+int64(len(metaResources)))
		for i := int64(0); i < size; i++ {
			typeRef += packedType[start+i]
			nameRef += packedName[start+i]

			if typeRef < 0 || typeRef >= int64(len(r.dictResourceStr)) ||
				nameRef < 0 || nameRef >= int64(len(r.dictResourceStr)) {
				return nil, errV3BadReference
			}

			resourcesSet = append(resourcesSet, &v3resource{r.dictResourceStr[typeRef], r.dictResourceStr[nameRef]})
		}

		if len(metaResources) > 0 {
			resourcesSet = append(resourcesSet, metaResources...)
		}

		resourcesDict = append(resourcesDict, resourcesSet)
		start = end
	}

	return resourcesDict, nil
}

func unpackOriginInfoDictV3(raw []int32) ([]*v3originInfo, error) {
	nelem := len(raw) / 3
	if len(raw) != nelem*3 {
		return nil, errV3UnexpectedEOF
	}
	dict := make([]*v3originInfo, 1, nelem+1)
	for i := 0; i < len(raw); i += 3 {
		dict = append(dict, &v3originInfo{int32(raw[i+0]), int32(raw[i+1]), int32(raw[i+2])})
	}

	return dict, nil
}

// HaveMoreMetrics returns true if there are more metrics to read.
func (r *metricReaderV3) HaveMoreMetrics() bool {
	return r.metricIdx < len(r.payload.MetricData.Types)
}

// NextMetric consumes next metric entry and prepares data for access.
//
// If this method returns an error the reader is in an invalid state and calling data access methods may panic.
func (r *metricReaderV3) NextMetric() error {
	if !r.HaveMoreMetrics() {
		return errV3UnexpectedEOF
	}

	if r.metricIdx >= 0 {
		for r.HaveMorePoints() {
			if err := r.NextPoint(); err != nil {
				return err
			}
		}
	}

	r.metricIdx++

	if r.metricIdx > len(r.payload.MetricData.Types) ||
		r.metricIdx > len(r.payload.MetricData.NameRefs) ||
		r.metricIdx > len(r.payload.MetricData.TagsetRefs) ||
		r.metricIdx > len(r.payload.MetricData.ResourcesRefs) ||
		r.metricIdx > len(r.payload.MetricData.SourceTypeNameRefs) ||
		r.metricIdx > len(r.payload.MetricData.OriginInfoRefs) ||
		r.metricIdx > len(r.payload.MetricData.Intervals) ||
		r.metricIdx > len(r.payload.MetricData.NumPoints) {
		return errV3UnexpectedEOF
	}

	r.pointsRemaining = int(r.v3NumPoints())

	r.nameRef += r.payload.MetricData.NameRefs[r.metricIdx-1]
	if r.nameRef < 0 || r.nameRef >= int64(len(r.dictNameStr)) {
		return errV3BadReference
	}

	r.tagsRef += r.payload.MetricData.TagsetRefs[r.metricIdx-1]
	if r.tagsRef < 0 || r.tagsRef >= int64(len(r.dictTagsets)) {
		return errV3BadReference
	}

	if r.HasUnit() {
		r.unitRefIdx++
		if r.unitRefIdx > len(r.payload.MetricData.UnitRefs) {
			return errV3UnexpectedEOF
		}
		r.unitRef += r.payload.MetricData.UnitRefs[r.unitRefIdx-1]
		if r.unitRef < 0 || r.unitRef >= int64(len(r.dictUnitStr)) {
			return errV3BadReference
		}
	}

	r.resourcesRef += r.payload.MetricData.ResourcesRefs[r.metricIdx-1]
	if r.resourcesRef < 0 || r.resourcesRef >= int64(len(r.dictResources)) {
		return errV3BadReference
	}

	r.sourceTypeNameRef += r.payload.MetricData.SourceTypeNameRefs[r.metricIdx-1]
	if r.sourceTypeNameRef < 0 || r.sourceTypeNameRef >= int64(len(r.dictSourceTypeName)) {
		return errV3BadReference
	}

	r.originInfoRef += r.payload.MetricData.OriginInfoRefs[r.metricIdx-1]
	if r.originInfoRef < 0 || r.originInfoRef >= int64(len(r.dictOriginInfo)) {
		return errV3BadReference
	}

	return nil
}

func (r *metricReaderV3) packedType() uint64 {
	return r.payload.MetricData.Types[r.metricIdx-1]
}

// Type returns type of current metric entry.
func (r *metricReaderV3) Type() intake_v3.MetricType {
	return intake_v3.MetricType(r.packedType() & 0xF)
}

// ValueType returns value type of current metric entry.
func (r *metricReaderV3) ValueType() intake_v3.ValueType {
	return intake_v3.ValueType(r.packedType() & 0xF0)
}

// Unit returns unit of current metric entry, or empty string if none.
func (r *metricReaderV3) Unit() string {
	if r.HasUnit() {
		return r.dictUnitStr[r.unitRef]
	}
	return ""
}

// Name returns metric name of current metric entry.
func (r *metricReaderV3) Name() string {
	return r.dictNameStr[r.nameRef]
}

// Tags returns set of tags for current metric entry.
func (r *metricReaderV3) Tags() []string {
	return r.dictTagsets[r.tagsRef]
}

// Resources returns set of resources for current metric entry.
//
//nolint:revive
func (r *metricReaderV3) Resources() []*v3resource {
	return r.dictResources[r.resourcesRef]
}

// SourceTypeName returns source type identifier for current metric entry.
func (r *metricReaderV3) SourceTypeName() string {
	return r.dictSourceTypeName[r.sourceTypeNameRef]
}

// Origin returns product origin information for current metric entry.
//
//nolint:revive
func (r *metricReaderV3) Origin() *v3originInfo {
	return r.dictOriginInfo[r.originInfoRef]
}

// Interval returns metric time interval for current metric entry.
func (r *metricReaderV3) Interval() uint64 {
	return r.payload.MetricData.Intervals[r.metricIdx-1]
}

// v3NumPoints returns number of data points contained in the current metric entry.
func (r *metricReaderV3) v3NumPoints() uint64 {
	return r.payload.MetricData.NumPoints[r.metricIdx-1]
}

// NoIndex returns true if the metric should not be indexed.
func (r *metricReaderV3) NoIndex() bool {
	return r.packedType()&uint64(intake_v3.MetricFlags_flagNoIndex) != 0
}

// HaveMorePoints returns true if there are more points to read.
func (r *metricReaderV3) HaveMorePoints() bool {
	return r.pointsRemaining > 0
}

// HasUnit returns true if the current metric entry has a unit.
func (r *metricReaderV3) HasUnit() bool {
	return r.packedType()&uint64(intake_v3.MetricFlags_flagHasUnit) != 0
}

// NextPoint consumes next unread metric data point and prepares data for access.
//
// If this method returns an error the reader is in an invalid state and calling data access methods may panic.
func (r *metricReaderV3) NextPoint() error {
	if !r.HaveMorePoints() {
		return errV3UnexpectedEOF
	}

	r.pointIdx++
	r.pointsRemaining--

	if r.pointIdx > len(r.payload.MetricData.Timestamps) {
		return errV3UnexpectedEOF
	}

	switch r.Type() {
	case intake_v3.MetricType_Sketch:
		r.sketchNumBinsIdx++
		if r.sketchNumBinsIdx > len(r.payload.MetricData.SketchNumBins) {
			return errV3UnexpectedEOF
		}
		r.sketchBinsIdx += r.SketchNumBins()
		switch r.ValueType() {
		case intake_v3.ValueType_Float64:
			r.valsFloat64Idx += 3
			r.valsSint64Idx++
		case intake_v3.ValueType_Float32:
			r.valsFloat32Idx += 3
			r.valsSint64Idx++
		case intake_v3.ValueType_Sint64:
			r.valsSint64Idx += 4
		case intake_v3.ValueType_Zero:
			r.valsSint64Idx++
		}
	default:
		switch r.ValueType() {
		case intake_v3.ValueType_Float64:
			r.valsFloat64Idx++
		case intake_v3.ValueType_Float32:
			r.valsFloat32Idx++
		case intake_v3.ValueType_Sint64:
			r.valsSint64Idx++
		}
	}

	if r.valsFloat64Idx > len(r.payload.MetricData.ValsFloat64) {
		return errV3UnexpectedEOF
	}
	if r.valsFloat32Idx > len(r.payload.MetricData.ValsFloat32) {
		return errV3UnexpectedEOF
	}
	if r.valsSint64Idx > len(r.payload.MetricData.ValsSint64) {
		return errV3UnexpectedEOF
	}
	if r.sketchBinsIdx > len(r.payload.MetricData.SketchBinKeys) {
		return errV3UnexpectedEOF
	}
	if r.sketchBinsIdx > len(r.payload.MetricData.SketchBinCnts) {
		return errV3UnexpectedEOF
	}

	r.timestamp += r.payload.MetricData.Timestamps[r.pointIdx-1]

	return nil
}

// Timestamp returns timestamp for current metric data point.
func (r *metricReaderV3) Timestamp() int64 {
	return r.timestamp
}

// Value returns metric value for current metric data point.
//
// Only valid to call if r.Type() != MetricType_Sketch, panics otherwise.
func (r *metricReaderV3) Value() float64 {
	if r.Type() == intake_v3.MetricType_Sketch {
		panic("invalid type")
	}
	switch r.ValueType() {
	case intake_v3.ValueType_Float64:
		return r.payload.MetricData.ValsFloat64[r.valsFloat64Idx-1]
	case intake_v3.ValueType_Float32:
		return float64(r.payload.MetricData.ValsFloat32[r.valsFloat32Idx-1])
	case intake_v3.ValueType_Sint64:
		return float64(r.payload.MetricData.ValsSint64[r.valsSint64Idx-1])
	default:
		return 0
	}
}

// SketchSummary returns sketch summary for current metric data point.
//
// Only valid if r.Type() == MetricType_Sketch, panics otherwise.
func (r *metricReaderV3) SketchSummary() (sum, min, max float64, cnt uint64) {
	if r.Type() != intake_v3.MetricType_Sketch {
		panic("invalid type")
	}

	cnt = uint64(r.payload.MetricData.ValsSint64[r.valsSint64Idx-1])

	switch r.ValueType() {
	case intake_v3.ValueType_Zero:
	case intake_v3.ValueType_Sint64:
		sum = float64(r.payload.MetricData.ValsSint64[r.valsSint64Idx-4])
		min = float64(r.payload.MetricData.ValsSint64[r.valsSint64Idx-3])
		max = float64(r.payload.MetricData.ValsSint64[r.valsSint64Idx-2])
		// -1 is cnt
	case intake_v3.ValueType_Float32:
		sum = float64(r.payload.MetricData.ValsFloat32[r.valsFloat32Idx-3])
		min = float64(r.payload.MetricData.ValsFloat32[r.valsFloat32Idx-2])
		max = float64(r.payload.MetricData.ValsFloat32[r.valsFloat32Idx-1])
	case intake_v3.ValueType_Float64:
		sum = r.payload.MetricData.ValsFloat64[r.valsFloat64Idx-3]
		min = r.payload.MetricData.ValsFloat64[r.valsFloat64Idx-2]
		max = r.payload.MetricData.ValsFloat64[r.valsFloat64Idx-1]
	}

	return
}

// SketchNumBins returns number of sketch bins for the current metric data point.
//
// Only valid if r.Type() == MetricType_Sketch, panics otherwise.
func (r *metricReaderV3) SketchNumBins() int {
	if r.Type() != intake_v3.MetricType_Sketch {
		panic("invalid type")
	}
	return int(r.payload.MetricData.SketchNumBins[r.sketchNumBinsIdx-1])
}

// SketchCols returns sketch data columns for the current metric data
// point.
//
// Only valid if r.Type() == MetricType_Sketch, panics otherwise.
func (r *metricReaderV3) SketchCols() (k []int32, n []uint32) {
	if r.Type() != intake_v3.MetricType_Sketch {
		panic("invalid type")
	}
	size := r.SketchNumBins()
	start := r.sketchBinsIdx - size
	k = slices.Clone(r.payload.MetricData.SketchBinKeys[start:][:size])
	n = slices.Clone(r.payload.MetricData.SketchBinCnts[start:][:size])
	deltaDecodeV3(k)
	return
}

func deltaDecodeV3(s []int32) {
	for i := 1; i < len(s); i++ {
		s[i] += s[i-1]
	}
}

func (r *metricReaderV3) Debug() {
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
		r.metricIdx,
		r.pointIdx,
		r.valsSint64Idx,
		r.valsFloat32Idx,
		r.valsFloat64Idx,
		r.sketchNumBinsIdx,
		r.sketchBinsIdx,

		r.pointsRemaining,

		r.nameRef,
		r.tagsRef,
		r.resourcesRef,
		r.sourceTypeNameRef,
		r.originInfoRef,
		r.timestamp,
	)

}
