// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

//
// NOTE: This module contains a feature in development that is NOT supported.
//

package percentile

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	log "github.com/cihub/seelog"
)

// EPSILON represents the accuracy of the sketch.
const EPSILON float64 = 0.01

// Entry is an element of the sketch. For the definition of g and delta, see the original paper
// http://infolab.stanford.edu/~datar/courses/cs361a/papers/quantiles.pdf
type Entry struct {
	V     float64 `json:"v"`
	G     uint32  `json:"g"`
	Delta uint32  `json:"d"`
}

//Entries is a slice of Entry
type Entries []Entry

func (slice Entries) Len() int           { return len(slice) }
func (slice Entries) Less(i, j int) bool { return slice[i].V < slice[j].V }
func (slice Entries) Swap(i, j int)      { slice[i], slice[j] = slice[j], slice[i] }

// GKArray is a version of GK with a buffer for the incoming values.
// The expected usage is that Add() is called in the agent, while Merge() and
// Quantile() are called in the backend; in other words, values are added to
// the sketch in the agent, sketches are sent to the backend where they are
// merged with other sketches, and quantile queries are made to the merged
// sketches only. This allows us to ignore the Incoming buffer once the
// sketch goes through a Merge.
// GKArray therefore has two versions of compress:
// 1. compressWithIncoming(incomingEntries []Entry) is used during Merge(), and sets
//	Incoming to nil after compressing so that merged sketches do not allocate
//	unnecessary storage.
// 2. compressAndAllocateBuf() is used during Add(), and allocates Incoming after
//	compressing for further addition of values to the sketch.
type GKArray struct {
	// the last item of Entries will always be the max inserted value
	Entries  Entries   `json:"entries"`
	Incoming []float64 `json:"buf"`
	// TODO[Charles]: incorporate min in entries so that we can get rid of the field.
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Count int64   `json:"cnt"`
	Sum   float64 `json:"sum"`
	Avg   float64 `json:"avg"`
}

func marshalEntries(entries Entries) ([]float64, []uint32, []uint32) {
	v := make([]float64, 0, len(entries))
	g := make([]uint32, 0, len(entries))
	delta := make([]uint32, 0, len(entries))
	for _, e := range entries {
		v = append(v, e.V)
		g = append(g, e.G)
		delta = append(delta, e.Delta)
	}
	return v, g, delta
}

func unmarshalEntries(v []float64, g []uint32, delta []uint32) Entries {
	entries := make(Entries, 0, len(v))
	for i := 0; i < len(v); i++ {
		entries = append(entries, Entry{V: v[i], G: g[i], Delta: delta[i]})
	}
	return entries
}

// MarshalJSON encodes an Entry into an array of values
func (e *Entry) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[%v, %v, %v]", e.V, e.G, e.Delta)), nil
}

// UnmarshalJSON decodes an Entry from an array of values
func (e *Entry) UnmarshalJSON(b []byte) error {
	values := [3]float64{}
	if err := json.Unmarshal(b, &values); err != nil {
		return err
	}
	e.V = values[0]
	e.G = uint32(values[1])
	e.Delta = uint32(values[2])
	return nil
}

// NewGKArray allocates a new GKArray summary.
func NewGKArray() GKArray {
	return GKArray{
		// preallocate the incoming array for better insert throughput (5% faster)
		Incoming: make([]float64, 0, int(1/EPSILON)+1),
		Min:      math.Inf(1),
		Max:      math.Inf(-1),
	}
}

// Add a new value to the summary.
func (s GKArray) Add(v float64) GKArray {
	s.Count++
	s.Sum += v
	s.Avg += (v - s.Avg) / float64(s.Count)
	s.Incoming = append(s.Incoming, v)
	if v < s.Min {
		s.Min = v
	}
	if v > s.Max {
		s.Max = v
	}
	if s.Count%(int64(1/EPSILON)+1) == 0 {
		return s.compressAndAllocateBuf()
	}

	return s
}

// compressAndAllocateBuf compresses Incoming into Entries, then allocates
// an empty Incoming for further addition of values.
func (s GKArray) compressAndAllocateBuf() GKArray {
	s = s.compressWithIncoming(nil)
	// allocate Incoming
	s.Incoming = make([]float64, 0, int(1/EPSILON)+1)
	return s
}

// Quantile returns an epsilon estimate of the element at quantile q.
// The incoming buffer should be empty during the quantile query phase,
// so the check for Incoming/Compress() should not run.
func (s GKArray) Quantile(q float64) float64 {
	if q < 0 || q > 1 {
		log.Errorf("Quantile out of bounds")
		return math.NaN()
	}

	if s.Count == 0 {
		return math.NaN()
	}

	// This shouldn't happen but checking just in case.
	if len(s.Incoming) > 0 {
		s = s.compressWithIncoming(nil)
	}

	// Interpolate the quantile when there are only a few values.
	if s.Count < int64(1/EPSILON) {
		return s.interpolatedQuantile(q)
	}

	rank := int64(q * float64(s.Count-1))
	spread := int64(EPSILON * float64(s.Count-1))
	gSum := int64(0)
	i := 0
	for ; i < len(s.Entries); i++ {
		gSum += int64(s.Entries[i].G)
		// mininum rank is 0 but gSum starts from 1, hence the -1.
		if gSum+int64(s.Entries[i].Delta)-1 > rank+spread {
			break
		}
	}
	if i == 0 {
		return s.Min
	}
	return s.Entries[i-1].V
}

// interpolatedQuantile returns an estimate of the element at quantile q,
// but interpolates between the lower and higher elements when Count is
// less than 1/EPSILON.
// Again, the incoming buffer is empty during the quantile query phase.
func (s GKArray) interpolatedQuantile(q float64) float64 {
	rank := q * float64(s.Count-1)
	indexBelow := int64(rank)
	indexAbove := indexBelow + 1
	if indexAbove > s.Count-1 {
		indexAbove = s.Count - 1
	}
	weightAbove := rank - float64(indexBelow)
	weightBelow := 1.0 - weightAbove

	// When Count is less than 1/EPSILON, all the entries will have G = 1, Delta = 0.
	return weightBelow*s.Entries[indexBelow].V + weightAbove*s.Entries[indexAbove].V
}

// Quantiles accepts a sorted slice of quantile values and returns the epsilon estimates of
// elements at those quantiles.
func (s GKArray) Quantiles(qValues []float64) []float64 {
	quantiles := make([]float64, 0, len(qValues))
	if s.Count == 0 {
		for range qValues {
			quantiles = append(quantiles, math.NaN())
		}
		return quantiles
	}

	// This shouldn't happen but checking just in case.
	if len(s.Incoming) > 0 {
		s = s.compressWithIncoming(nil)
	}

	// When there are only a few values just call interpolatedQuantile
	// for each quantile.
	if s.Count < int64(1/EPSILON) {
		for _, q := range qValues {
			if q < 0 || q > 1 {
				quantiles = append(quantiles, math.NaN())
				continue
			}
			quantiles = append(quantiles, s.interpolatedQuantile(q))
		}
		return quantiles
	}

	// If the qValues are not sorted, just call Quantile for each qValue
	if !sort.Float64sAreSorted(qValues) {
		for k, q := range qValues {
			quantiles[k] = s.Quantile(q)
		}
		return quantiles
	}

	// For sorted qValues, the quantiles can be found in one pass
	// over the Entries
	spread := int64(EPSILON * float64(s.Count-1))
	gSum := int64(0)
	i := 0
	j := 0
	for ; i < len(s.Entries) && j < len(qValues); i++ {
		gSum += int64(s.Entries[i].G)
		// Check for invalid qValues
		for ; j < len(qValues) && (qValues[j] < 0 || qValues[j] > 1); j++ {
			quantiles = append(quantiles, math.NaN())
		}
		// Loop since adjacent ranks could be the same.
		for ; j < len(qValues) && gSum+int64(s.Entries[i].Delta)-1 > int64(qValues[j]*float64(s.Count-1))+spread; j++ {
			if i == 0 {
				quantiles = append(quantiles, s.Min)
			} else {
				quantiles = append(quantiles, s.Entries[i-1].V)
			}
		}
	}
	// If there're any quantile values that have not been found,
	// return the max value.
	for ; j < len(qValues); j++ {
		if qValues[j] < 0 || qValues[j] > 1 {
			quantiles = append(quantiles, math.NaN())
			continue
		}
		quantiles = append(quantiles, s.Max)
	}
	return quantiles
}

// Merge another GKArray into this in-place.
func (s GKArray) Merge(o GKArray) GKArray {
	if o.Count == 0 {
		return s.compressWithIncoming(nil)
	}
	if s.Count == 0 {
		return o.compressWithIncoming(nil)
	}
	o = o.compressWithIncoming(nil)
	spread := uint32(EPSILON * float64(o.Count-1))

	/*
			Here is one way to merge summaries so that the sketch is one-way mergeable: we extract an epsilon-approximate
			distribution from one of the summaries (o) and we insert this distribution into the other summary (s). More
			specifically, to extract the approximate distribution, we can query for all the quantiles i/(o.valCount-1) where i
			is between 0 and o.Count-1 (included). Then we insert those values into s as usual. This way, when querying a
			quantile from the merged summary, the returned quantile has a rank error from the inserted values that is lower than
			epsilon, but the inserted values, because of the merge process, have a rank error from the actual data that is also
			lower than epsilon, so that the total rank error is bounded by 2*epsilon.
			However, querying and inserting each value as described above has a complexity that is linear in the number of
			values that have been inserted in o rather than in the number of entries in the summary. To tackle this issue, we
			can notice that each of the quantiles that are queried from o is a v of one of the entry of o. Instead of actually
			querying for those quantiles, we can count the number of times each v will be returned (when querying the quantiles
		        i/(o.valCount-1)); we end up with the values n below. Then instead of successively inserting each v n times, we can
			actually directly append them to s.Incoming as new entries where g = n. This is possible because the values of n
			will never violate the condition n <= int(s.eps * (s.Count+o.Count-1)). Also, we need to make sure that
			compress() can handle entries in Incoming where g > 1.
	*/

	IncomingEntries := make([]Entry, 0, len(o.Entries)+1)
	if n := o.Entries[0].G + o.Entries[0].Delta - spread - 1; n > 0 {
		IncomingEntries = append(IncomingEntries, Entry{V: o.Min, G: n, Delta: 0})
	}
	for i := 0; i < len(o.Entries)-1; i++ {
		if n := o.Entries[i+1].G + o.Entries[i+1].Delta - o.Entries[i].Delta; n > 0 { // TODO[Charles]: is the check necessary?
			IncomingEntries = append(IncomingEntries, Entry{V: o.Entries[i].V, G: n, Delta: 0})
		}
	}
	if n := spread + 1 - o.Entries[len(o.Entries)-1].Delta; n > 0 { // TODO[Charles]: is the check necessary?
		IncomingEntries = append(IncomingEntries, Entry{V: o.Entries[len(o.Entries)-1].V, G: n, Delta: 0})
	}

	s.Count += o.Count
	s.Sum += o.Sum
	s.Avg = s.Avg + (o.Avg-s.Avg)*float64(o.Count)/float64(s.Count)
	if o.Min < s.Min {
		s.Min = o.Min
	}
	if o.Max > s.Max {
		s.Max = o.Max
	}
	return s.compressWithIncoming(IncomingEntries)
}

// compressWithIncoming merges an optional incomingEntries and Incoming buffer into
// Entries and compresses. Incoming buffer is set to nil after compressing.
func (s GKArray) compressWithIncoming(incomingEntries Entries) GKArray {
	// TODO[Charles]: use s.Incoming and incomingEntries directly instead of merging them prior to compressing
	if len(s.Incoming) > 0 {
		incomingCopy := make([]Entry, len(incomingEntries), len(incomingEntries)+len(s.Incoming))
		copy(incomingCopy, incomingEntries)
		incomingEntries = incomingCopy
		for _, v := range s.Incoming {
			incomingEntries = append(incomingEntries, Entry{V: v, G: 1, Delta: 0})
		}
	}
	sort.Sort(incomingEntries)

	// Copy Entries slice so as not to change the original
	entriesCopy := make([]Entry, len(s.Entries), len(s.Entries))
	copy(entriesCopy, s.Entries)
	s.Entries = entriesCopy

	removalThreshold := 2 * uint32(EPSILON*float64(s.Count-1))
	merged := make([]Entry, 0, len(s.Entries)+len(incomingEntries)/3)

	// TODO[Charles]: The compression algo might not be optimal. We need to revisit it if we need to improve space
	// complexity (e.g., by compressing incoming entries).
	i, j := 0, 0
	for i < len(incomingEntries) || j < len(s.Entries) {
		if i == len(incomingEntries) {
			// done with Incoming; now only considering the sketch
			if j+1 < len(s.Entries) &&
				s.Entries[j].G+s.Entries[j+1].G+s.Entries[j+1].Delta <= removalThreshold {
				// removable from sketch
				s.Entries[j+1].G += s.Entries[j].G
			} else {
				merged = append(merged, s.Entries[j])
			}
			j++
		} else if j == len(s.Entries) {
			// done with sketch; now only considering Incoming
			if i+1 < len(incomingEntries) &&
				incomingEntries[i].G+incomingEntries[i+1].G+incomingEntries[i+1].Delta <= removalThreshold {
				// removable from Incoming
				incomingEntries[i+1].G += incomingEntries[i].G
			} else {
				merged = append(merged, incomingEntries[i])
			}
			i++
		} else if incomingEntries[i].V < s.Entries[j].V {
			if incomingEntries[i].G+s.Entries[j].G+s.Entries[j].Delta <= removalThreshold {
				// removable from Incoming
				s.Entries[j].G += incomingEntries[i].G
			} else {
				incomingEntries[i].Delta = s.Entries[j].G + s.Entries[j].Delta - incomingEntries[i].G
				merged = append(merged, incomingEntries[i])
			}
			i++
		} else {
			if j+1 < len(s.Entries) &&
				s.Entries[j].G+s.Entries[j+1].G+s.Entries[j+1].Delta <= removalThreshold {
				// removable from sketch
				s.Entries[j+1].G += s.Entries[j].G
			} else {
				merged = append(merged, s.Entries[j])
			}
			j++
		}
	}
	s.Entries = merged
	// set Incoming to nil, since it is not used after merge
	s.Incoming = nil
	return s
}

// IsValid checks that the object is a minimally valid GKArray, i.e., won't
// cause a panic when calling Add or Merge.
func (s GKArray) IsValid() bool {
	// Check that Count is valid
	if s.Count < 0 {
		return false
	}
	if len(s.Entries) == 0 {
		if int64(len(s.Incoming)) != s.Count {
			return false
		}
	}
	gSum := int64(0)
	for _, e := range s.Entries {
		gSum += int64(e.G)
	}
	if gSum+int64(len(s.Incoming)) != s.Count {
		return false
	}
	return true
}
