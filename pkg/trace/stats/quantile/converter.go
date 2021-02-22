// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"errors"
	"sort"

	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/golang/protobuf/proto"
)

// ddSketch represents the sketch described here: http://www.vldb.org/pvldb/vol12/p2195-masson.pdf
// This representation only supports positive values.
type ddSketch struct {
	// bins is the map from index to count
	bins map[int32]float64
	// contiguousBins is a more compact representation for contiguous bins.
	// the index of each bin is the index in the array + contiguousBinsOffset
	contiguousBins       []float64
	contiguousBinsOffset int
	// zeros is the count of 0 and its close neighbors (close defined by gamma).
	zeros int
	// mapping is the mapping of the ddSketch: interpolation, gamma and global index offset.
	mapping mapping.IndexMapping
}

// count returns the count for a given index.
func (s *ddSketch) count(index int) (count int) {
	if index >= s.contiguousBinsOffset && index < s.contiguousBinsOffset+len(s.contiguousBins) {
		count = int(s.contiguousBins[index-s.contiguousBinsOffset])
	}
	if c, ok := s.bins[int32(index)]; ok {
		count += int(c)
	}
	return count
}

func (s *ddSketch) maxSize() int {
	return len(s.bins) + len(s.contiguousBins)
}

// getIndexes returns all the sorted indexes contained in s1 and s2.
func getIndexes(s1 ddSketch, s2 ddSketch) []int {
	// TODO(piochelepiotr): No need to re-allocate that array at each conversion.
	// but this function needs to be thread safe in the agent.
	indexes := make([]int, 0, s1.maxSize()+s2.maxSize())
	for i := range s1.contiguousBins {
		indexes = append(indexes, i+s1.contiguousBinsOffset)
	}
	for i := range s2.contiguousBins {
		index := i + s2.contiguousBinsOffset
		if index >= s1.contiguousBinsOffset && index < s1.contiguousBinsOffset+len(s1.contiguousBins) {
			continue
		}
		indexes = append(indexes, index)
	}
	for i := range s1.bins {
		index := int(i)
		if index >= s1.contiguousBinsOffset && index < s1.contiguousBinsOffset+len(s1.contiguousBins) {
			continue
		}
		if index >= s2.contiguousBinsOffset && index < s2.contiguousBinsOffset+len(s2.contiguousBins) {
			continue
		}
		indexes = append(indexes, index)
	}
	for i := range s2.bins {
		index := int(i)
		if index >= s1.contiguousBinsOffset && index < s1.contiguousBinsOffset+len(s1.contiguousBins) {
			continue
		}
		if index >= s2.contiguousBinsOffset && index < s2.contiguousBinsOffset+len(s2.contiguousBins) {
			continue
		}
		if _, ok := s1.bins[i]; ok {
			continue
		}
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes
}

// decodeDDSketch decodes a ddSketch from a protobuf encoded ddSketch.
// It only supports sketches with positive values.
func decodeDDSketch(data []byte) (ddSketch, error) {
	var pb sketchpb.DDSketch
	if err := proto.Unmarshal(data, &pb); err != nil {
		return ddSketch{}, err
	}
	mapping, err := mapping.FromProto(pb.Mapping)
	if err != nil {
		return ddSketch{}, err
	}
	if len(pb.NegativeValues.BinCounts) > 0 ||
		len(pb.NegativeValues.ContiguousBinCounts) > 0 {
		return ddSketch{}, errors.New("negative values not supported")
	}
	return ddSketch{
		mapping:              mapping,
		bins:                 pb.PositiveValues.BinCounts,
		contiguousBins:       pb.PositiveValues.ContiguousBinCounts,
		contiguousBinsOffset: int(pb.PositiveValues.ContiguousBinIndexOffset),
		zeros:                int(pb.ZeroCount),
	}, nil
}

// DDToGKSketches converts two dd sketches: ok and errors to 2 gk sketches: hits and errors
// with hits = ok + errors
func DDToGKSketches(okSketchData []byte, errSketchData []byte) (hits, errors *SliceSummary, err error) {
	okDDSketch, err := decodeDDSketch(okSketchData)
	if err != nil {
		return nil, nil, err
	}
	errDDSketch, err := decodeDDSketch(errSketchData)
	if err != nil {
		return nil, nil, err
	}

	hits = &SliceSummary{Entries: make([]Entry, 0, okDDSketch.maxSize())}
	errors = &SliceSummary{Entries: make([]Entry, 0, errDDSketch.maxSize())}
	if zeros := okDDSketch.zeros + errDDSketch.zeros; zeros > 0 {
		hits.Entries = append(hits.Entries, Entry{V: 0, G: zeros, Delta: 0})
		hits.N = zeros
	}
	if zeros := errDDSketch.zeros; zeros > 0 {
		errors.Entries = append(errors.Entries, Entry{V: 0, G: zeros, Delta: 0})
		errors.N = zeros
	}
	indexes := getIndexes(okDDSketch, errDDSketch)
	for _, index := range indexes {
		gErr := errDDSketch.count(index)
		gHits := okDDSketch.count(index) + gErr
		if gHits == 0 {
			// gHits == 0 implies gErr == 0
			continue
		}
		hits.N += gHits
		v := okDDSketch.mapping.Value(index)
		hits.Entries = append(hits.Entries, Entry{
			V:     v,
			G:     gHits,
			Delta: int(2 * EPSILON * float64(hits.N-1)),
		})
		if gErr == 0 {
			continue
		}
		errors.N += gErr
		errors.Entries = append(errors.Entries, Entry{
			V:     v,
			G:     gErr,
			Delta: int(2 * EPSILON * float64(errors.N-1)),
		})
	}
	if hits.N > 0 {
		hits.Entries[0].Delta = 0
		hits.Entries[len(hits.Entries)-1].Delta = 0
	}
	if errors.N > 0 {
		errors.Entries[0].Delta = 0
		errors.Entries[len(errors.Entries)-1].Delta = 0
	}
	return hits, errors, nil
}
