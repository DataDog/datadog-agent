// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package percentile

import (
	"math"
	"math/rand"
	"sort"

	agentpayload "github.com/DataDog/agent-payload/gogen"
)

// KLL is a probabilistic quantile sketch structure
type KLL struct {
	Compactors [][]float64 `json:"compactors"`
	Min        float64     `json:"min"`
	Max        float64     `json:"max"`
	Count      int64       `json:"cnt"`
	Sum        float64     `json:"sum"`
	Avg        float64     `json:"avg"`
	Length     uint32      `json:"len"`
	Capacity   uint32      `json:"cap"`
	H          uint32      `json:"h"`
}

// NewKLL allocates a KLL summary.
func NewKLL() KLL {
	kll := KLL{
		Min: math.Inf(1),
		Max: math.Inf(-1),
	}
	return kll.grow()
}

func marshalCompactors(compactors [][]float64) []agentpayload.SketchPayload_Sketch_DistributionK_Compactor {
	payload := make([]agentpayload.SketchPayload_Sketch_DistributionK_Compactor, 0, len(compactors))
	for _, c := range compactors {
		payload = append(payload, agentpayload.SketchPayload_Sketch_DistributionK_Compactor{V: c})
	}
	return payload
}

// Add a value to the sketch.
func (s KLL) Add(v float64) QSketch {
	s.Count++
	s.Sum += v
	s.Avg += (v - s.Avg) / float64(s.Count)
	if v < s.Min {
		s.Min = v
	}
	if v > s.Max {
		s.Max = v
	}
	s.Compactors[0] = append(s.Compactors[0], v)
	s.Length++
	if s.Length >= s.Capacity {
		s = s.compress()
		if s.Length >= s.Capacity {
			panic("Length is higher than expected.")
		}
	}
	return QSketch(s)
}

// Quantile returns an epsilon estimate of the element at quantile q.
func (s KLL) Quantile(q float64) float64 {
	if q < 0 || q > 1 {
		panic("quantile out of bounds")
	}
	if s.Count == 0 {
		return math.NaN()
	}
	if q == 0 {
		return s.Min
	} else if q == 1 {
		return s.Max
	} else if s.Count < int64(1/EPSILON) {
		return s.interpolatedQuantile(q)
	}
	rank := int(q * float64(s.Count-1))
	return s.findQuantile(rank, s.Min, s.Max)
}

func (s KLL) interpolatedQuantile(q float64) float64 {
	rank := q * float64(s.Count-1)
	indexBelow := int64(rank)
	indexAbove := indexBelow + 1
	if indexAbove > s.Count-1 {
		indexAbove = s.Count - 1
	}
	weightAbove := rank - float64(indexBelow)
	weightBelow := 1.0 - weightAbove

	sort.Float64s(s.Compactors[0])
	return weightBelow*s.Compactors[0][indexBelow] + weightAbove*s.Compactors[0][indexAbove]
}

// Merge merges the receiver sketch with another, inplace.
func (s KLL) Merge(o KLL) KLL {
	// Grow until self has at least as many compactors as other
	for s.H < o.H {
		s = s.grow()
	}
	// Append the items in same height compactors
	for height := 0; height < int(o.H); height++ {
		s.Compactors[height] = append(s.Compactors[height], o.Compactors[height]...)
	}
	// Some bookkeeping
	s.Length += o.Length
	//	for _, c := range s.Compactors {
	//		s.ksize += len(c)
	//	}
	s.Count += o.Count
	s.Sum += o.Sum
	s.Avg += (o.Avg - s.Avg) * float64(o.Count) / float64(s.Count)
	if o.Min < s.Min {
		s.Min = o.Min
	}
	if o.Max > s.Max {
		s.Max = o.Max
	}
	// Keep compressing until the size constraint is met
	for s.Length >= s.Capacity {
		s = s.compress()
	}
	return s
}

func (s KLL) findQuantile(rank int, minVal, maxVal float64) float64 {
	qval := (maxVal + minVal) / 2
	qrank, lowerQuantile, higherQuantile := s.rankAndQuantiles(qval)
	if qrank >= rank {
		if lowerQuantile > minVal {
			// keep going, we can be more accurate
			return s.findQuantile(rank, minVal, lowerQuantile)
		}
		// the rank of qval may be higher than expected but there is nothing between minVal and qval
		return lowerQuantile
	}
	if higherQuantile < maxVal {
		// keep going, we can be more accurate
		return s.findQuantile(rank, higherQuantile, maxVal)
	}
	// the rank of qval is lower than expected but there is nothing between qval and maxVal
	return higherQuantile
}

// rankAndQuantiles returns an estimate of the rank of the value as well as the lower and higher quantiles.
func (s KLL) rankAndQuantiles(query float64) (int, float64, float64) {
	rank := -1
	lowerQuantile := s.Min
	higherQuantile := s.Max
	for h, c := range s.Compactors {
		for _, item := range c {
			if item <= query {
				rank += 1 << uint(h)
				if item > lowerQuantile {
					lowerQuantile = item
				}
			}
			if query <= item && item < higherQuantile {
				higherQuantile = item
			}
		}
	}
	return rank, lowerQuantile, higherQuantile
}

func (s KLL) compress() KLL {
	for h := range s.Compactors {
		if uint32(len(s.Compactors[h])) >= s.findCapacity(h) {
			if h+1 >= int(s.H) {
				s = s.grow()
			}
			compacted := s.compact(h)

			s.Compactors[h+1] = append(s.Compactors[h+1], compacted...)
			s.Compactors[h] = []float64{}

			// compute the new size
			s.Length = 0
			for _, c := range s.Compactors {
				s.Length += uint32(len(c))
			}
			// Here we return because we reduced the ksize by at least 1
			return s
		}
	}
	return s
}

func (s KLL) compact(index int) []float64 {
	sort.Float64s(s.Compactors[index])
	i := 0
	if rand.Float64() < .5 {
		i = 1
	}
	compacted := make([]float64, 0, len(s.Compactors[index])/2+1)
	for ; i < len(s.Compactors[index]); i += 2 {
		compacted = append(compacted, s.Compactors[index][i])
	}
	return compacted
}

func (s KLL) grow() KLL {
	s.Compactors = append(s.Compactors, []float64{})
	s.H = uint32(len(s.Compactors))
	s.Capacity = 0
	for height := 0; height < int(s.H); height++ {
		s.Capacity += s.findCapacity(height)
	}
	return s
}

func (s KLL) findCapacity(height int) uint32 {
	c := 2. / 3.
	depth := int(s.H) - height - 1
	return uint32(math.Ceil(math.Pow(c, float64(depth))*1/EPSILON)) + 1
}
