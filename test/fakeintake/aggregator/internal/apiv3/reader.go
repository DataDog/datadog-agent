// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package apiv3 provides a reader for the V3 metrics intake wire format.
//
// Copied for use in fakeintake. Update when reader.go changes.
package apiv3

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

// Reader is an iterator over data contained in Payload.
//
// Usage:
//
//	r := NewReader(payload.Payload)
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
//	        // Accessors for metric data point can be called.
//	    }
//	 }
type Reader struct {
	payload *Payload

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
	dictResources      [][]*resource
	dictSourceTypeName []string
	dictOriginInfo     []*originInfo
}

type resource = [2]string
type originInfo = [3]int32

// NewReader creates an iterator over the data contained in MetricData
// by wrapping it in a Payload
func NewReader(data *MetricData) *Reader {
	return &Reader{
		payload: &Payload{
			MetricData: data,
		},
	}
}

// NewPayloadReader creates an iterator over the data contained in Payload
func NewPayloadReader(payload *Payload) *Reader {
	return &Reader{
		payload: payload,
	}
}

// Initialize reads and normalizes payload dictionaries for fast access.
// This method must be called before any other method on the reader.
func (r *Reader) Initialize() error {
	if r.payload.MetricData == nil {
		return errors.New("metric data must not be nil")
	}

	var err error
	r.dictNameStr, err = unpackStrDict(r.payload.MetricData.DictNameStr, false)
	if err != nil {
		return err
	}
	r.dictTagsStr, err = unpackStrDict(r.payload.MetricData.DictTagStr, true)
	if err != nil {
		return err
	}
	r.dictUnitStr, err = unpackStrDict(r.payload.MetricData.DictUnitStr, false)
	if err != nil {
		return err
	}
	r.dictTagsets, err = r.unpackTagsetsDict()
	if err != nil {
		return err
	}
	r.dictResourceStr, err = unpackStrDict(r.payload.MetricData.DictResourceStr, false)
	if err != nil {
		return err
	}
	r.dictResources, err = r.unpackResourcesDict()
	if err != nil {
		return err
	}
	r.dictSourceTypeName, err = unpackStrDict(r.payload.MetricData.DictSourceTypeName, false)
	if err != nil {
		return err
	}
	r.dictOriginInfo, err = unpackOriginInfoDict(r.payload.MetricData.DictOriginInfo)
	if err != nil {
		return err
	}
	return nil
}

var (
	errUnexpectedEOF = errors.New("unexpected end of column")
	errOverflow      = errors.New("length field overflow")
	errBadReference  = errors.New("invalid reference")
	errInvalidUTF8   = errors.New("invalid UTF-8 string")
)

func unpackStrDict(raw []byte, sanitizeInvalidUTF8 bool) ([]string, error) {
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

		if !utf8.ValidString(str) {
			if sanitizeInvalidUTF8 {
				str = strings.ToValidUTF8(str, string(utf8.RuneError))
			} else {
				return nil, errInvalidUTF8
			}
		}

		dict = append(dict, str)
		raw = raw[end:]
	}
	return dict, nil
}

func (r *Reader) unpackTagsetsDict() ([][]string, error) {
	packed := r.payload.MetricData.DictTagsets
	tagsets := [][]string{nil}

	metadataTags := r.payload.GetMetadata().GetTags()

	for len(packed) > 0 {
		size := packed[0]
		packed = packed[1:]
		if size < 0 || size > int64(len(packed)) {
			return nil, errUnexpectedEOF
		}
		tags := make([]string, 0, int(size)+len(metadataTags))

		idx := int64(0)
		for i := int64(0); i < size; i++ {
			idx += packed[i]

			if idx < 0 {
				if idx <= -math.MaxInt64 || -idx >= int64(len(tagsets)) {
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

func (r *Reader) unpackResourcesDict() ([][]*resource, error) {
	packedLen := r.payload.MetricData.DictResourceLen
	packedType := r.payload.MetricData.DictResourceType
	packedName := r.payload.MetricData.DictResourceName
	resourcesDict := make([][]*resource, 1, len(packedLen)+1)

	metadataResources := r.payload.GetMetadata().GetResources()

	// Decode metadata resources once
	var metaResources []*resource
	if len(metadataResources) > 0 {
		if len(metadataResources)%2 != 0 {
			return nil, errors.New("metadata resources must be [Type, Name] pairs")
		}
		pairs := len(metadataResources) / 2
		metaResources = make([]*resource, pairs)
		for i := 0; i < pairs; i++ {
			t := metadataResources[2*i]
			n := metadataResources[2*i+1]
			metaResources[i] = &resource{t, n}
		}
	}

	start := int64(0)
	for _, size := range packedLen {
		if size < 0 {
			return nil, errUnexpectedEOF
		}
		if size > math.MaxInt64-start {
			return nil, errOverflow
		}
		end := start + size
		if end > int64(len(packedType)) || end > int64(len(packedName)) {
			return nil, errBadReference
		}

		typeRef := int64(0)
		nameRef := int64(0)
		resourcesSet := make([]*resource, 0, size+int64(len(metaResources)))
		for i := int64(0); i < size; i++ {
			typeRef += packedType[start+i]
			nameRef += packedName[start+i]

			if typeRef < 0 || typeRef >= int64(len(r.dictResourceStr)) ||
				nameRef < 0 || nameRef >= int64(len(r.dictResourceStr)) {
				return nil, errBadReference
			}

			resourcesSet = append(resourcesSet, &resource{r.dictResourceStr[typeRef], r.dictResourceStr[nameRef]})
		}

		if len(metaResources) > 0 {
			resourcesSet = append(resourcesSet, metaResources...)
		}

		resourcesDict = append(resourcesDict, resourcesSet)
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
		dict = append(dict, &originInfo{int32(raw[i+0]), int32(raw[i+1]), int32(raw[i+2])})
	}

	return dict, nil
}

// HaveMoreMetrics returns true if there are more metrics to read.
func (r *Reader) HaveMoreMetrics() bool {
	return r.metricIdx < len(r.payload.MetricData.Types)
}

// NextMetric consumes next metric entry and prepares data for access.
//
// If this method returns an error the reader is in an invalid state and calling data access methods may panic.
func (r *Reader) NextMetric() error {
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

	if r.metricIdx > len(r.payload.MetricData.Types) ||
		r.metricIdx > len(r.payload.MetricData.NameRefs) ||
		r.metricIdx > len(r.payload.MetricData.TagsetRefs) ||
		r.metricIdx > len(r.payload.MetricData.ResourcesRefs) ||
		r.metricIdx > len(r.payload.MetricData.SourceTypeNameRefs) ||
		r.metricIdx > len(r.payload.MetricData.OriginInfoRefs) ||
		r.metricIdx > len(r.payload.MetricData.Intervals) ||
		r.metricIdx > len(r.payload.MetricData.NumPoints) {
		return errUnexpectedEOF
	}

	r.pointsRemaining = int(r.NumPoints())

	r.nameRef += r.payload.MetricData.NameRefs[r.metricIdx-1]
	if r.nameRef < 0 || r.nameRef >= int64(len(r.dictNameStr)) {
		return errBadReference
	}

	r.tagsRef += r.payload.MetricData.TagsetRefs[r.metricIdx-1]
	if r.tagsRef < 0 || r.tagsRef >= int64(len(r.dictTagsets)) {
		return errBadReference
	}

	if r.HasUnit() {
		r.unitRefIdx++
		if r.unitRefIdx > len(r.payload.MetricData.UnitRefs) {
			return errUnexpectedEOF
		}
		r.unitRef += r.payload.MetricData.UnitRefs[r.unitRefIdx-1]
		if r.unitRef < 0 || r.unitRef >= int64(len(r.dictUnitStr)) {
			return errBadReference
		}
	}

	r.resourcesRef += r.payload.MetricData.ResourcesRefs[r.metricIdx-1]
	if r.resourcesRef < 0 || r.resourcesRef >= int64(len(r.dictResources)) {
		return errBadReference
	}

	r.sourceTypeNameRef += r.payload.MetricData.SourceTypeNameRefs[r.metricIdx-1]
	if r.sourceTypeNameRef < 0 || r.sourceTypeNameRef >= int64(len(r.dictSourceTypeName)) {
		return errBadReference
	}

	r.originInfoRef += r.payload.MetricData.OriginInfoRefs[r.metricIdx-1]
	if r.originInfoRef < 0 || r.originInfoRef >= int64(len(r.dictOriginInfo)) {
		return errBadReference
	}

	return nil
}

func (r *Reader) packedType() uint64 {
	return r.payload.MetricData.Types[r.metricIdx-1]
}

// Type returns type of current metric entry.
func (r *Reader) Type() MetricType {
	return MetricType(r.packedType() & 0xF)
}

// ValueType returns value type of current metric entry.
func (r *Reader) ValueType() ValueType {
	return ValueType(r.packedType() & 0xF0)
}

// Unit returns unit of current metric entry, or empty string if none.
func (r *Reader) Unit() string {
	if r.HasUnit() {
		return r.dictUnitStr[r.unitRef]
	}
	return ""
}

// Name returns metric name of current metric entry.
func (r *Reader) Name() string {
	return r.dictNameStr[r.nameRef]
}

// Tags returns set of tags for current metric entry.
func (r *Reader) Tags() []string {
	return r.dictTagsets[r.tagsRef]
}

// Resources returns set of resources for current metric entry.
//
//nolint:revive
func (r *Reader) Resources() []*resource {
	return r.dictResources[r.resourcesRef]
}

// SourceTypeName returns source type identifier for current metric entry.
func (r *Reader) SourceTypeName() string {
	return r.dictSourceTypeName[r.sourceTypeNameRef]
}

// Origin returns product origin information for current metric entry.
//
//nolint:revive
func (r *Reader) Origin() *originInfo {
	return r.dictOriginInfo[r.originInfoRef]
}

// Interval returns metric time interval for current metric entry.
func (r *Reader) Interval() uint64 {
	return r.payload.MetricData.Intervals[r.metricIdx-1]
}

// NumPoints returns number of data points contained in the current metric entry.
func (r *Reader) NumPoints() uint64 {
	return r.payload.MetricData.NumPoints[r.metricIdx-1]
}

// NoIndex returns true if the metric should not be indexed.
func (r *Reader) NoIndex() bool {
	return r.packedType()&uint64(MetricFlags_flagNoIndex) != 0
}

// HaveMorePoints returns true if there are more points to read.
func (r *Reader) HaveMorePoints() bool {
	return r.pointsRemaining > 0
}

// HasUnit returns true if the current metric entry has a unit.
func (r *Reader) HasUnit() bool {
	return r.packedType()&uint64(MetricFlags_flagHasUnit) != 0
}

// NextPoint consumes next unread metric data point and prepares data for access.
//
// If this method returns an error the reader is in an invalid state and calling data access methods may panic.
func (r *Reader) NextPoint() error {
	if !r.HaveMorePoints() {
		return errUnexpectedEOF
	}

	r.pointIdx++
	r.pointsRemaining--

	if r.pointIdx > len(r.payload.MetricData.Timestamps) {
		return errUnexpectedEOF
	}

	switch r.Type() {
	case MetricType_Sketch:
		r.sketchNumBinsIdx++
		if r.sketchNumBinsIdx > len(r.payload.MetricData.SketchNumBins) {
			return errUnexpectedEOF
		}
		r.sketchBinsIdx += r.SketchNumBins()
		switch r.ValueType() {
		case ValueType_Float64:
			r.valsFloat64Idx += 3
			r.valsSint64Idx++
		case ValueType_Float32:
			r.valsFloat32Idx += 3
			r.valsSint64Idx++
		case ValueType_Sint64:
			r.valsSint64Idx += 4
		case ValueType_Zero:
			r.valsSint64Idx++
		}
	default:
		switch r.ValueType() {
		case ValueType_Float64:
			r.valsFloat64Idx++
		case ValueType_Float32:
			r.valsFloat32Idx++
		case ValueType_Sint64:
			r.valsSint64Idx++
		}
	}

	if r.valsFloat64Idx > len(r.payload.MetricData.ValsFloat64) {
		return errUnexpectedEOF
	}
	if r.valsFloat32Idx > len(r.payload.MetricData.ValsFloat32) {
		return errUnexpectedEOF
	}
	if r.valsSint64Idx > len(r.payload.MetricData.ValsSint64) {
		return errUnexpectedEOF
	}
	if r.sketchBinsIdx > len(r.payload.MetricData.SketchBinKeys) {
		return errUnexpectedEOF
	}
	if r.sketchBinsIdx > len(r.payload.MetricData.SketchBinCnts) {
		return errUnexpectedEOF
	}

	r.timestamp += r.payload.MetricData.Timestamps[r.pointIdx-1]

	return nil
}

// Timestamp returns timestamp for current metric data point.
func (r *Reader) Timestamp() int64 {
	return r.timestamp
}

// Value returns metric value for current metric data point.
//
// Only valid to call if r.Type() != MetricType_Sketch, panics otherwise.
func (r *Reader) Value() float64 {
	if r.Type() == MetricType_Sketch {
		panic("invalid type")
	}
	switch r.ValueType() {
	case ValueType_Float64:
		return r.payload.MetricData.ValsFloat64[r.valsFloat64Idx-1]
	case ValueType_Float32:
		return float64(r.payload.MetricData.ValsFloat32[r.valsFloat32Idx-1])
	case ValueType_Sint64:
		return float64(r.payload.MetricData.ValsSint64[r.valsSint64Idx-1])
	default:
		return 0
	}
}

// SketchSummary returns sketch summary for current metric data point.
//
// Only valid if r.Type() == MetricType_Sketch, panics otherwise.
func (r *Reader) SketchSummary() (sum, min, max float64, cnt uint64) {
	if r.Type() != MetricType_Sketch {
		panic("invalid type")
	}

	cnt = uint64(r.payload.MetricData.ValsSint64[r.valsSint64Idx-1])

	switch r.ValueType() {
	case ValueType_Zero:
	case ValueType_Sint64:
		sum = float64(r.payload.MetricData.ValsSint64[r.valsSint64Idx-4])
		min = float64(r.payload.MetricData.ValsSint64[r.valsSint64Idx-3])
		max = float64(r.payload.MetricData.ValsSint64[r.valsSint64Idx-2])
		// -1 is cnt
	case ValueType_Float32:
		sum = float64(r.payload.MetricData.ValsFloat32[r.valsFloat32Idx-3])
		min = float64(r.payload.MetricData.ValsFloat32[r.valsFloat32Idx-2])
		max = float64(r.payload.MetricData.ValsFloat32[r.valsFloat32Idx-1])
	case ValueType_Float64:
		sum = r.payload.MetricData.ValsFloat64[r.valsFloat64Idx-3]
		min = r.payload.MetricData.ValsFloat64[r.valsFloat64Idx-2]
		max = r.payload.MetricData.ValsFloat64[r.valsFloat64Idx-1]
	}

	return
}

// SketchNumBins returns number of sketch bins for the current metric data point.
//
// Only valid if r.Type() == MetricType_Sketch, panics otherwise.
func (r *Reader) SketchNumBins() int {
	if r.Type() != MetricType_Sketch {
		panic("invalid type")
	}
	return int(r.payload.MetricData.SketchNumBins[r.sketchNumBinsIdx-1])
}

// SketchCols returns sketch data columns for the current metric data
// point.
//
// Only valid if r.Type() == MetricType_Sketch, panics otherwise.
func (r *Reader) SketchCols() (k []int32, n []uint32) {
	if r.Type() != MetricType_Sketch {
		panic("invalid type")
	}
	size := r.SketchNumBins()
	start := r.sketchBinsIdx - size
	k = slices.Clone(r.payload.MetricData.SketchBinKeys[start:][:size])
	n = slices.Clone(r.payload.MetricData.SketchBinCnts[start:][:size])
	deltaDecode(k)
	return
}

func deltaDecode(s []int32) {
	for i := 1; i < len(s); i++ {
		s[i] += s[i-1]
	}
}

func (r *Reader) Debug() {
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
