package quantile

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/davecgh/go-spew/spew"
	"github.com/gogo/protobuf/proto"
	"sort"
)

type ddSketch struct {
	contiguousBins []float64
	bins map[int32]float64
	offset int
	zeros int
	mapping mapping.IndexMapping
}

// get returns the count for a given index.
func (s *ddSketch) get(index int) (count int) {
	if index >= s.offset && index < s.offset + len(s.contiguousBins) {
		count = int(s.contiguousBins[index-s.offset])
	}
	if c, ok := s.bins[int32(index)]; ok {
		count += int(c)
	}
	return count
}

func (s *ddSketch) maxSize() int {
	return len(s.bins) + len(s.contiguousBins)
}

func getIndexes(s1 ddSketch, s2 ddSketch) []int {
	// todo: No need to re-allocate that array at each conversion.
	// but this function needs to be thread safe in the agent.
	indexes := make([]int, 0, s1.maxSize() + s2.maxSize())
	for i := range s1.contiguousBins {
		indexes = append(indexes, i + s1.offset)
	}
	for i := range s2.contiguousBins {
		ind := i + s2.offset
		if ind >= s1.offset && ind < s1.offset + len(s1.contiguousBins) {
			continue
		}
		indexes = append(indexes, ind)
	}
	for i := range s1.bins {
		ind := int(i)
		if ind >= s1.offset && ind < s1.offset + len(s1.contiguousBins) {
			continue
		}
		if ind >= s2.offset && ind < s2.offset + len(s2.contiguousBins) {
			continue
		}
		indexes = append(indexes, ind)
	}
	for i := range s2.bins {
		ind := int(i)
		if ind >= s1.offset && ind < s1.offset + len(s1.contiguousBins) {
			continue
		}
		if ind >= s2.offset && ind < s2.offset + len(s2.contiguousBins) {
			continue
		}
		if _, ok := s1.bins[i]; ok {
			continue
		}
		indexes = append(indexes, ind)
	}
	sort.Ints(indexes)
	return indexes
}

// decodeDDSketch decodes a ddSketch from a protobuf encoded ddSketch
// it only supports positive contiguous bins
func decodeDDSketch(data []byte) (ddSketch, error) {
	var sketchPb pb.DDSketch
	if err := proto.Unmarshal(data, &sketchPb); err != nil {
		return ddSketch{}, err
	}
	mapping, err := ddSketchMappingFromProto(sketchPb.Mapping)
	if err != nil {
		return ddSketch{}, err
	}
	if sketchPb.Mapping.IndexOffset > 0 { err = errors.New("index offset non 0") }
	if  len(sketchPb.NegativeValues.BinCounts)> 0 { err = errors.New("contains negative values") }
	if  len(sketchPb.NegativeValues.ContiguousBinCounts)> 0 { err = errors.New("contains negative values") }
	if err != nil {
		return ddSketch{}, errors.New("ddSketch format not supported: " + err.Error())
	}
	return ddSketch{
		mapping: mapping,
		bins: sketchPb.PositiveValues.BinCounts,
		contiguousBins: sketchPb.PositiveValues.ContiguousBinCounts,
		offset: int(sketchPb.PositiveValues.ContiguousBinIndexOffset),
		zeros: int(sketchPb.ZeroCount),
	}, nil
}

func ddSketchMappingFromProto(mappingPb *pb.IndexMapping) (m mapping.IndexMapping, err error) {
	switch mappingPb.Interpolation {
	case pb.IndexMapping_NONE:
		return mapping.NewLogarithmicMappingWithGamma(mappingPb.Gamma, mappingPb.IndexOffset)
	case pb.IndexMapping_LINEAR:
		return mapping.NewLinearlyInterpolatedMappingWithGamma(mappingPb.Gamma, mappingPb.IndexOffset)
	case pb.IndexMapping_CUBIC:
		return mapping.NewCubicallyInterpolatedMappingWithGamma(mappingPb.Gamma, mappingPb.IndexOffset)
	default:
		return nil, fmt.Errorf("interpolation not supported: %d", mappingPb.Interpolation)
	}
}

// DDToGKSketches converts two dd sketches: ok and errors to 2 gk sketches: hits and errors
// with hits = ok + errors
func DDToGKSketches(okSketchData []byte, errSketchData []byte) (hits, errors *SliceSummary, err error) {
	okDDSketch, err := decodeDDSketch(okSketchData)
	if err != nil {
		return nil, nil, err
	}
	// todo: remove dump
	fmt.Println("\nok sketch")
	spew.Dump(okDDSketch)
	errDDSketch, err := decodeDDSketch(errSketchData)
	if err != nil {
		return nil, nil, err
	}
	// todo: remove dump
	fmt.Println("\nerror sketch")
	spew.Dump(errDDSketch)

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
		gErr := errDDSketch.get(index)
		gHits := okDDSketch.get(index) + gErr
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
	hits.compress()
	errors.compress()
	return hits, errors, nil
}
