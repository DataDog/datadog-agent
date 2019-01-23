package agent

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// WeightedSpan extends Span to contain weights required by the Concentrator.
type WeightedSpan struct {
	Weight   float64 // Span weight. Similar to the trace root.Weight().
	TopLevel bool    // Is this span a service top-level or not. Similar to span.TopLevel().

	*pb.Span
}

// WeightedTrace is a slice of WeightedSpan pointers.
type WeightedTrace []*WeightedSpan

// NewWeightedTrace returns a weighted trace, with coefficient required by the concentrator.
func NewWeightedTrace(trace pb.Trace, root *pb.Span) WeightedTrace {
	wt := make(WeightedTrace, len(trace))

	weight := sampler.Weight(root)

	for i := range trace {
		wt[i] = &WeightedSpan{
			Span:     trace[i],
			Weight:   weight,
			TopLevel: traceutil.HasTopLevel(trace[i]),
		}
	}
	return wt
}
