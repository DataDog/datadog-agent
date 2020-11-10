package quantile

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// DDSketchToGK only support contiguousBins and log mapping
// for the moment
func DDSketchToGK(ddSketch *pb.DDSketch) *SliceSummary {
	gkSketch := SliceSummary{Entries: make([]Entry, 0, len(ddSketch.PositiveValues.ContiguousBinCounts))}
	gamma := ddSketch.Mapping.Gamma

	zeros := int(ddSketch.ZeroCount)
	if zeros > 0 {
		gkSketch.Entries = append(gkSketch.Entries, Entry{V: 0, G: zeros, Delta: 0})
	}
	indexOffset := int(ddSketch.PositiveValues.ContiguousBinIndexOffset)
	var total int
	for i, count := range ddSketch.PositiveValues.ContiguousBinCounts {
		index := i + indexOffset - 1
		g := int(count)
		if g == 0 {
			continue
		}
		total += g
		v := valueFromIndex(index, gamma)
		gkSketch.Entries = append(gkSketch.Entries, Entry{
			V:     v,
			G:     g,
			Delta: int(2 * EPSILON * float64(total-1)),
		})
	}
	gkSketch.N = total
	if len(gkSketch.Entries) > 0 {
		gkSketch.Entries[0].Delta = 0
		gkSketch.Entries[len(gkSketch.Entries)-1].Delta = 0
	}
	gkSketch.compress()
	return &gkSketch
}

func valueFromIndex(index int, gamma float64) float64 {
	return 2 * math.Exp(float64(index)*math.Log(gamma)) / (gamma + 1)
}
