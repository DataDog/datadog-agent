package stats

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Subtrace represents the combination of a root span and the trace consisting of all its descendant spans
type Subtrace struct {
	Root  *pb.Span
	Trace pb.Trace
}

// spanAndAncestors is used by ExtractTopLevelSubtraces to store the pair of a span and its ancestors
type spanAndAncestors struct {
	Span      *pb.Span
	Ancestors []*pb.Span
}

// element and queue implement a very basic LIFO used to do an iterative DFS on a trace
type element struct {
	SpanAndAncestors *spanAndAncestors
	Next             *element
}

type stack struct {
	head *element
}

func (s *stack) Push(value *spanAndAncestors) {
	e := &element{value, nil}
	if s.head == nil {
		s.head = e
		return
	}
	e.Next = s.head
	s.head = e
}

func (s *stack) Pop() *spanAndAncestors {
	if s.head == nil {
		return nil
	}
	value := s.head.SpanAndAncestors
	s.head = s.head.Next
	return value
}

// ExtractTopLevelSubtraces extracts all subtraces rooted in a toplevel span,
// ComputeTopLevel should be called before.
func ExtractTopLevelSubtraces(t pb.Trace, root *pb.Span) []Subtrace {
	if root == nil {
		return []Subtrace{}
	}
	childrenMap := traceutil.ChildrenMap(t)
	subtraces := []Subtrace{}

	visited := make(map[*pb.Span]bool, len(t))
	subtracesMap := make(map[*pb.Span][]*pb.Span)
	var next stack
	next.Push(&spanAndAncestors{root, []*pb.Span{}})

	// We do a DFS on the trace to record the toplevel ancesters of each span
	for current := next.Pop(); current != nil; current = next.Pop() {
		// We do not extract subtraces for toplevel spans that have no children
		// since these are not interresting
		if traceutil.HasTopLevel(current.Span) && len(childrenMap[current.Span.SpanID]) > 0 {
			current.Ancestors = append(current.Ancestors, current.Span)
		}
		visited[current.Span] = true
		for _, ancestor := range current.Ancestors {
			subtracesMap[ancestor] = append(subtracesMap[ancestor], current.Span)
		}
		for _, child := range childrenMap[current.Span.SpanID] {
			// Continue if this span has already been explored (meaning the
			// trace is not a Tree)
			if visited[child] {
				log.Warnf("Found a cycle while processing traceID:%v, trace should be a tree", t[0].TraceID)
				continue
			}
			next.Push(&spanAndAncestors{child, current.Ancestors})
		}
	}

	for topLevel, subtrace := range subtracesMap {
		subtraces = append(subtraces, Subtrace{topLevel, subtrace})
	}

	return subtraces
}
