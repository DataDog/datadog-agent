package quantile

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/davecgh/go-spew/spew"
	"github.com/gogo/protobuf/proto"
)

type ddSketch struct {
	bins []float64
	offset int
	zeros int
	mapping mapping.IndexMapping
}

func (s *ddSketch) get(index int) int {
	if index < s.offset || index >= s.offset + len(s.bins) {
		return 0
	}
	return int(s.bins[index-s.offset])
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// ddSketchToGK only support positive contiguous bins
func ddSketchToGK(okDDSketch ddSketch, errDDSketch ddSketch) (hits, errors *SliceSummary) {
	minOffset := min(okDDSketch.offset, errDDSketch.offset)
	maxIndex := max(okDDSketch.offset + len(okDDSketch.bins), errDDSketch.offset + len(errDDSketch.bins))
	hits = &SliceSummary{Entries: make([]Entry, 0, maxIndex - minOffset)}
	errors = &SliceSummary{Entries: make([]Entry, 0, len(errDDSketch.bins))}
	if zeros := okDDSketch.zeros + errDDSketch.zeros; zeros > 0 {
		hits.Entries = append(hits.Entries, Entry{V: 0, G: zeros, Delta: 0})
		hits.N = zeros
	}
	if zeros := errDDSketch.zeros; zeros > 0 {
		errors.Entries = append(errors.Entries, Entry{V: 0, G: zeros, Delta: 0})
		errors.N = zeros
	}
	for index := minOffset; index < maxIndex; index++ {
		gErr := errDDSketch.get(index)
		gHits := okDDSketch.get(index) + gErr
		if gHits == 0 {
			// if gHits == 0, gErr == 0 also
			continue
		}
		hits.N += gHits
		errors.N += gErr
		v := okDDSketch.mapping.Value(index)
		hits.Entries = append(hits.Entries, Entry{
			V:     v,
			G:     gHits,
			Delta: int(2 * EPSILON * float64(hits.N-1)),
		})
		if gErr == 0 {
			continue
		}
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
	return hits, errors
}

func getDDSketchMapping(protoMapping *pb.IndexMapping) (m mapping.IndexMapping, err error) {
	switch protoMapping.Interpolation {
	case pb.IndexMapping_NONE:
		return mapping.NewLogarithmicMappingWithGamma(protoMapping.Gamma, protoMapping.IndexOffset)
	case pb.IndexMapping_LINEAR:
		return mapping.NewLinearlyInterpolatedMappingWithGamma(protoMapping.Gamma, protoMapping.IndexOffset)
	case pb.IndexMapping_CUBIC:
		return mapping.NewCubicallyInterpolatedMappingWithGamma(protoMapping.Gamma, protoMapping.IndexOffset)
	default:
		return nil, fmt.Errorf("interpolation not supported: %d", protoMapping.Interpolation)
	}
}

func ddSketchFromData(data []byte) (ddSketch, error) {
	var sketchPb pb.DDSketch
	if err := proto.Unmarshal(data, &sketchPb); err != nil {
		return ddSketch{}, err
	}
	mapping, err := getDDSketchMapping(sketchPb.Mapping)
	if err != nil {
		return ddSketch{}, err
	}
	return ddSketch{
		mapping: mapping,
		bins: sketchPb.PositiveValues.ContiguousBinCounts,
		offset: int(sketchPb.PositiveValues.ContiguousBinIndexOffset),
		zeros: int(sketchPb.ZeroCount),
	}, nil
}

// DDSketchesToGK converts two dd sketches representing ok and errors to 2 gk sketches
// representing hits and errors, with hits = ok + errors
func DDSketchesToGK(okSummaryData []byte, errorSummaryData []byte) (hitsSketch *SliceSummary, errorSketch *SliceSummary, err error) {
	okDDSketch, err := ddSketchFromData(okSummaryData)
	if err != nil {
		return nil, nil, err
	}
	fmt.Println("\nok sketch")
	spew.Dump(okDDSketch)
	errDDSketch, err := ddSketchFromData(errorSummaryData)
	if err != nil {
		return nil, nil, err
	}
	fmt.Println("\nerror sketch")
	spew.Dump(errDDSketch)
	hits, errors := ddSketchToGK(okDDSketch, errDDSketch)
	return hits, errors, nil
}
