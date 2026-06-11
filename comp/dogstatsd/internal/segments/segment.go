// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package segments prototypes payload-aligned, sealed DogStatsD metric segments.
package segments

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

var (
	// ErrSegmentFull is returned when a builder reaches its row budget.
	ErrSegmentFull = errors.New("segment row budget exceeded")
)

// Row is the semantic metric row represented by a sealed segment.
type Row struct {
	Type           metrics.MetricType
	Name           string
	Tags           []string
	Resources      []metrics.Resource
	SourceTypeName string
	Origin         string
	Unit           string
	Timestamp      int64
	Value          float64
	NoIndex        bool
}

// Options configures a Builder.
type Options struct {
	MaxRows int
}

// Builder accumulates metric rows into payload-aligned dictionaries and columns.
type Builder struct {
	maxRows int

	nameDict           dictionary
	tagDict            dictionary
	tagsetDict         dictionary
	resourceStringDict dictionary
	resourceSetDict    dictionary
	sourceTypeDict     dictionary
	originDict         dictionary
	unitDict           dictionary

	columns segmentColumns
}

type dictionary struct {
	values []string
	index  map[string]uint32
}

type segmentColumns struct {
	types              []metrics.MetricType
	nameRefs           []uint32
	tagsetRefs         []uint32
	resourceSetRefs    []uint32
	sourceTypeNameRefs []uint32
	originRefs         []uint32
	unitRefs           []uint32
	timestamps         []int64
	values             []float64
	noIndex            []bool
}

// Segment is an immutable, payload-aligned metric block.
type Segment struct {
	NameDictionary           []string
	TagDictionary            []string
	Tagsets                  [][]uint32
	ResourceStringDictionary []string
	ResourceSets             [][]ResourceRef
	SourceTypeDictionary     []string
	OriginDictionary         []string
	UnitDictionary           []string

	columns segmentColumns
}

// ResourceRef references resource type/name strings in ResourceStringDictionary.
type ResourceRef struct {
	TypeRef uint32
	NameRef uint32
}

// Stats describes segment dictionary and row sizes.
type Stats struct {
	Rows            int
	Names           int
	TagStrings      int
	Tagsets         int
	ResourceStrings int
	ResourceSets    int
	SourceTypes     int
	Origins         int
	Units           int
}

// NewBuilder returns a new segment builder.
func NewBuilder(opts Options) *Builder {
	return &Builder{
		maxRows:            opts.MaxRows,
		nameDict:           newDictionary(),
		tagDict:            newDictionary(),
		tagsetDict:         newDictionary(),
		resourceStringDict: newDictionary(),
		resourceSetDict:    newDictionary(),
		sourceTypeDict:     newDictionary(),
		originDict:         newDictionary(),
		unitDict:           newDictionary(),
	}
}

// Add appends one semantic row to the segment under construction.
func (b *Builder) Add(row Row) error {
	if b.maxRows > 0 && len(b.columns.types) >= b.maxRows {
		return ErrSegmentFull
	}

	nameRef := b.nameDict.intern(row.Name)
	tagsetRef := b.internTagset(row.Tags)
	resourceSetRef := b.internResourceSet(row.Resources)
	sourceTypeRef := b.sourceTypeDict.intern(row.SourceTypeName)
	originRef := b.originDict.intern(row.Origin)
	unitRef := b.unitDict.intern(row.Unit)

	b.columns.types = append(b.columns.types, row.Type)
	b.columns.nameRefs = append(b.columns.nameRefs, nameRef)
	b.columns.tagsetRefs = append(b.columns.tagsetRefs, tagsetRef)
	b.columns.resourceSetRefs = append(b.columns.resourceSetRefs, resourceSetRef)
	b.columns.sourceTypeNameRefs = append(b.columns.sourceTypeNameRefs, sourceTypeRef)
	b.columns.originRefs = append(b.columns.originRefs, originRef)
	b.columns.unitRefs = append(b.columns.unitRefs, unitRef)
	b.columns.timestamps = append(b.columns.timestamps, row.Timestamp)
	b.columns.values = append(b.columns.values, row.Value)
	b.columns.noIndex = append(b.columns.noIndex, row.NoIndex)
	return nil
}

// Seal returns an immutable segment copy.
func (b *Builder) Seal() Segment {
	return Segment{
		NameDictionary:           cloneStrings(b.nameDict.values),
		TagDictionary:            cloneStrings(b.tagDict.values),
		Tagsets:                  cloneUint32Matrix(b.tagsets()),
		ResourceStringDictionary: cloneStrings(b.resourceStringDict.values),
		ResourceSets:             cloneResourceRefMatrix(b.resourceSets()),
		SourceTypeDictionary:     cloneStrings(b.sourceTypeDict.values),
		OriginDictionary:         cloneStrings(b.originDict.values),
		UnitDictionary:           cloneStrings(b.unitDict.values),
		columns:                  b.columns.clone(),
	}
}

// Rows reconstructs semantic metric rows from the sealed segment.
func (s Segment) Rows() []Row {
	rows := make([]Row, 0, len(s.columns.types))
	for i := range s.columns.types {
		rows = append(rows, Row{
			Type:           s.columns.types[i],
			Name:           s.NameDictionary[s.columns.nameRefs[i]],
			Tags:           s.tagsForRef(s.columns.tagsetRefs[i]),
			Resources:      s.resourcesForRef(s.columns.resourceSetRefs[i]),
			SourceTypeName: s.SourceTypeDictionary[s.columns.sourceTypeNameRefs[i]],
			Origin:         s.OriginDictionary[s.columns.originRefs[i]],
			Unit:           s.UnitDictionary[s.columns.unitRefs[i]],
			Timestamp:      s.columns.timestamps[i],
			Value:          s.columns.values[i],
			NoIndex:        s.columns.noIndex[i],
		})
	}
	return rows
}

// Stats returns dictionary and row sizes for the segment.
func (s Segment) Stats() Stats {
	return Stats{
		Rows:            len(s.columns.types),
		Names:           len(s.NameDictionary),
		TagStrings:      len(s.TagDictionary),
		Tagsets:         len(s.Tagsets),
		ResourceStrings: len(s.ResourceStringDictionary),
		ResourceSets:    len(s.ResourceSets),
		SourceTypes:     len(s.SourceTypeDictionary),
		Origins:         len(s.OriginDictionary),
		Units:           len(s.UnitDictionary),
	}
}

// NameRef returns the payload-local name dictionary reference for row i.
func (s Segment) NameRef(i int) uint32 { return s.columns.nameRefs[i] }

// TagsetRef returns the payload-local tagset dictionary reference for row i.
func (s Segment) TagsetRef(i int) uint32 { return s.columns.tagsetRefs[i] }

// OriginRef returns the payload-local origin dictionary reference for row i.
func (s Segment) OriginRef(i int) uint32 { return s.columns.originRefs[i] }

func newDictionary() dictionary {
	return dictionary{index: make(map[string]uint32)}
}

func (d *dictionary) intern(value string) uint32 {
	if ref, ok := d.index[value]; ok {
		return ref
	}
	ref := uint32(len(d.values))
	d.values = append(d.values, value)
	d.index[value] = ref
	return ref
}

func (b *Builder) internTagset(tags []string) uint32 {
	parts := make([]string, 0, len(tags))
	for _, tag := range tags {
		parts = append(parts, fmt.Sprint(b.tagDict.intern(tag)))
	}
	key := strings.Join(parts, ",")
	return b.tagsetDict.intern(key)
}

func (b *Builder) internResourceSet(resources []metrics.Resource) uint32 {
	refs := make([]ResourceRef, 0, len(resources))
	parts := make([]string, 0, len(resources)*2)
	for _, resource := range resources {
		typeRef := b.resourceStringDict.intern(resource.Type)
		nameRef := b.resourceStringDict.intern(resource.Name)
		refs = append(refs, ResourceRef{TypeRef: typeRef, NameRef: nameRef})
		parts = append(parts, fmt.Sprint(typeRef), fmt.Sprint(nameRef))
	}
	key := strings.Join(parts, ",")
	if ref, ok := b.resourceSetDict.index["resource:"+key]; ok {
		return ref
	}
	ref := uint32(len(b.resourceSetDict.values))
	b.resourceSetDict.values = append(b.resourceSetDict.values, "resource:"+key)
	b.resourceSetDict.index["resource:"+key] = ref
	return ref
}

func (b *Builder) tagsets() [][]uint32 {
	tagsets := make([][]uint32, len(b.tagsetDict.values))
	for key, ref := range b.tagsetDict.index {
		tagsets[ref] = parseUint32Refs(key)
	}
	return compactUint32Matrix(tagsets)
}

func (b *Builder) resourceSets() [][]ResourceRef {
	sets := make([][]ResourceRef, len(b.resourceSetDict.values))
	for key, ref := range b.resourceSetDict.index {
		if !strings.HasPrefix(key, "resource:") {
			continue
		}
		sets[ref] = parseResourceRefs(strings.TrimPrefix(key, "resource:"))
	}
	return compactResourceRefMatrix(sets)
}

func (s Segment) tagsForRef(ref uint32) []string {
	if len(s.Tagsets) == 0 {
		return nil
	}
	refs := s.Tagsets[ref]
	tags := make([]string, 0, len(refs))
	for _, tagRef := range refs {
		tags = append(tags, s.TagDictionary[tagRef])
	}
	return tags
}

func (s Segment) resourcesForRef(ref uint32) []metrics.Resource {
	if len(s.ResourceSets) == 0 {
		return nil
	}
	refs := s.ResourceSets[ref]
	resources := make([]metrics.Resource, 0, len(refs))
	for _, resourceRef := range refs {
		resources = append(resources, metrics.Resource{
			Type: s.ResourceStringDictionary[resourceRef.TypeRef],
			Name: s.ResourceStringDictionary[resourceRef.NameRef],
		})
	}
	return resources
}

func (c segmentColumns) clone() segmentColumns {
	return segmentColumns{
		types:              append([]metrics.MetricType(nil), c.types...),
		nameRefs:           append([]uint32(nil), c.nameRefs...),
		tagsetRefs:         append([]uint32(nil), c.tagsetRefs...),
		resourceSetRefs:    append([]uint32(nil), c.resourceSetRefs...),
		sourceTypeNameRefs: append([]uint32(nil), c.sourceTypeNameRefs...),
		originRefs:         append([]uint32(nil), c.originRefs...),
		unitRefs:           append([]uint32(nil), c.unitRefs...),
		timestamps:         append([]int64(nil), c.timestamps...),
		values:             append([]float64(nil), c.values...),
		noIndex:            append([]bool(nil), c.noIndex...),
	}
}

func cloneStrings(in []string) []string { return append([]string(nil), in...) }

func cloneUint32Matrix(in [][]uint32) [][]uint32 {
	out := make([][]uint32, len(in))
	for i := range in {
		out[i] = append([]uint32(nil), in[i]...)
	}
	return out
}

func cloneResourceRefMatrix(in [][]ResourceRef) [][]ResourceRef {
	out := make([][]ResourceRef, len(in))
	for i := range in {
		out[i] = append([]ResourceRef(nil), in[i]...)
	}
	return out
}

func parseUint32Refs(value string) []uint32 {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	refs := make([]uint32, 0, len(parts))
	for _, part := range parts {
		var ref uint32
		fmt.Sscan(part, &ref)
		refs = append(refs, ref)
	}
	return refs
}

func parseResourceRefs(value string) []ResourceRef {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	refs := make([]ResourceRef, 0, len(parts)/2)
	for i := 0; i+1 < len(parts); i += 2 {
		var typeRef uint32
		var nameRef uint32
		fmt.Sscan(parts[i], &typeRef)
		fmt.Sscan(parts[i+1], &nameRef)
		refs = append(refs, ResourceRef{TypeRef: typeRef, NameRef: nameRef})
	}
	return refs
}

func compactUint32Matrix(in [][]uint32) [][]uint32 {
	last := -1
	for i := range in {
		if in[i] != nil {
			last = i
		}
	}
	if last < 0 {
		return [][]uint32{{}}
	}
	return in[:last+1]
}

func compactResourceRefMatrix(in [][]ResourceRef) [][]ResourceRef {
	last := -1
	for i := range in {
		if in[i] != nil {
			last = i
		}
	}
	if last < 0 {
		return [][]ResourceRef{{}}
	}
	return in[:last+1]
}
