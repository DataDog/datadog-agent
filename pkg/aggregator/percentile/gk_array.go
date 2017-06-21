package percentile

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// EPSILON represents the accuracy of the sketch.
const EPSILON float64 = 0.01

// Entry is an element of the sketch. For the definition of g and delta, see the original paper
// http://infolab.stanford.edu/~datar/courses/cs361a/papers/quantiles.pdf
type Entry struct {
	V     float64 `json:"v"`
	G     int     `json:"g"`
	Delta int     `json:"d"`
}

//Entries is a slice of Entry
type Entries []Entry

func (slice Entries) Len() int           { return len(slice) }
func (slice Entries) Less(i, j int) bool { return slice[i].V < slice[j].V }
func (slice Entries) Swap(i, j int)      { slice[i], slice[j] = slice[j], slice[i] }

// GKArray is a version of GK with a buffer for the incoming values.
type GKArray struct {
	// the last item of Entries will always be the max inserted value
	Entries  Entries `json:"entries"`
	incoming []float64
	// TODO[Charles]: incorporate min in entries so that we can get rid of the field.
	Min      float64 `json:"min"`
	ValCount int     `json:"n"`
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
	e.G = int(values[1])
	e.Delta = int(values[2])
	return nil
}

// NewGKArray allocates a new GKArray summary.
func NewGKArray() GKArray {
	return GKArray{
		// preallocate the incoming array for better insert throughput (5% faster)
		incoming: make([]float64, 0, int(1/EPSILON)),
		Min:      math.Inf(1),
	}
}

// Add a new value to the summary.
func (s *GKArray) Add(v float64) {
	s.ValCount++
	s.incoming = append(s.incoming, v)
	if v < s.Min {
		s.Min = v
	}
	if s.ValCount%int(1/EPSILON) == 0 {
		s.compress(nil)
	}
}

// Quantile returns an epsilon estimate of the element at quantile q.
func (s *GKArray) Quantile(q float64) float64 {
	if q < 0 || q > 1 {
		panic("Quantile out of bounds")
	}

	if s.ValCount == 0 {
		return math.NaN()
	}

	// Interpolate the quantile when there are only a few values.
	if s.ValCount < int(1/EPSILON) {
		return s.interpolatedQuantile(q)
	}

	if len(s.Entries) == 0 {
		sort.Float64s(s.incoming)
		return s.incoming[int(q*float64(s.ValCount-1))]
	}

	if len(s.incoming) > 0 {
		s.compress(nil)
	}

	rank := int(q * float64(s.ValCount-1))
	spread := int(EPSILON * float64(s.ValCount-1))
	gSum := 0
	i := 0
	for ; i < len(s.Entries); i++ {
		gSum += s.Entries[i].G
		// mininum rank is 0 but gSum starts from 1, hence the -1.
		if gSum+s.Entries[i].Delta-1 > rank+spread {
			break
		}
	}
	if i == 0 {
		return s.Min
	}
	return s.Entries[i-1].V
}

// interpolatedQuantile returns an estimate of the element at quantile q,
// but interpolates between the lower and higher elements when ValCount is
// less than 1/EPSILON
func (s *GKArray) interpolatedQuantile(q float64) float64 {
	rank := q * float64(s.ValCount-1)
	indexBelow := int(rank)
	indexAbove := indexBelow + 1
	if indexAbove > s.ValCount-1 {
		indexAbove = s.ValCount - 1
	}
	weightAbove := rank - float64(indexBelow)
	weightBelow := 1.0 - weightAbove

	if len(s.incoming) > 0 {
		s.compress(nil)
	}
	// When ValCount is less than 1/EPSILON, all the entries will have G = 1, Delta = 0.
	return weightBelow*s.Entries[indexBelow].V + weightAbove*s.Entries[indexAbove].V
}

// Merge another GKArray into this in-place.
func (s *GKArray) Merge(o GKArray) {
	if o.ValCount == 0 {
		return
	}
	if s.ValCount == 0 {
		s.Entries = o.Entries
		s.ValCount = o.ValCount
		s.incoming = o.incoming
		s.Min = o.Min
		return
	}
	o.compress(nil)
	spread := int(EPSILON * float64(o.ValCount-1))

	/*
			Here is one way to merge summaries so that the sketch is one-way mergeable: we extract an epsilon-approximate
			distribution from one of the summaries (o) and we insert this distribution into the other summary (s). More
			specifically, to extract the approximate distribution, we can query for all the quantiles i/(o.valCount-1) where i
			is between 0 and o.ValCount-1 (included). Then we insert those values into s as usual. This way, when querying a
			quantile from the merged summary, the returned quantile has a rank error from the inserted values that is lower than
			epsilon, but the inserted values, because of the merge process, have a rank error from the actual data that is also
			lower than epsilon, so that the total rank error is bounded by 2*epsilon.
			However, querying and inserting each value as described above has a complexity that is linear in the number of
			values that have been inserted in o rather than in the number of entries in the summary. To tackle this issue, we
			can notice that each of the quantiles that are queried from o is a v of one of the entry of o. Instead of actually
			querying for those quantiles, we can count the number of times each v will be returned (when querying the quantiles
		        i/(o.valCount-1)); we end up with the values n below. Then instead of successively inserting each v n times, we can
			actually directly append them to s.incoming as new entries where g = n. This is possible because the values of n
			will never violate the condition n <= int(s.eps * (s.ValCount+o.ValCount-1)). Also, we need to make sure that
			compress() can handle entries in incoming where g > 1.
	*/

	incomingEntries := make([]Entry, 0, len(o.Entries))
	if n := o.Entries[0].G + o.Entries[0].Delta - spread - 1; n > 0 {
		incomingEntries = append(incomingEntries, Entry{V: o.Min, G: n, Delta: 0})
	}
	for i := 0; i < len(o.Entries)-1; i++ {
		if n := o.Entries[i+1].G + o.Entries[i+1].Delta - o.Entries[i].Delta; n > 0 { // TODO[Charles]: is the check necessary?
			incomingEntries = append(incomingEntries, Entry{V: o.Entries[i].V, G: n, Delta: 0})
		}
	}
	if n := spread + 1 - o.Entries[len(o.Entries)-1].Delta; n > 0 { // TODO[Charles]: is the check necessary?
		incomingEntries = append(incomingEntries, Entry{V: o.Entries[len(o.Entries)-1].V, G: n, Delta: 0})
	}

	s.ValCount += o.ValCount
	if o.Min < s.Min {
		s.Min = o.Min
	}
	s.compress(incomingEntries)
}

// Compress merges the incoming buffer into entries and compresses it.
func (s *GKArray) Compress() {
	if len(s.incoming) == 0 {
		return
	}
	s.compress(nil)
}

func (s *GKArray) compress(incomingEntries Entries) {

	// TODO[Charles]: use s.incoming and incomingEntries directly instead of merging them prior to compressing
	for _, v := range s.incoming {
		incomingEntries = append(incomingEntries, Entry{V: v, G: 1, Delta: 0})
	}
	sort.Sort(incomingEntries)

	removalThreshold := 2 * int(EPSILON*float64(s.ValCount-1))
	merged := make([]Entry, 0, len(s.Entries)+len(incomingEntries))

	// TODO[Charles]: The compression algo might not be optimal. We need to revisit it if we need to improve space
	// complexity (e.g., by compressing incoming entries).
	i, j := 0, 0
	for i < len(incomingEntries) || j < len(s.Entries) {
		if i == len(incomingEntries) {
			// done with incoming; now only considering the sketch
			if j+1 < len(s.Entries) &&
				s.Entries[j].G+s.Entries[j+1].G+s.Entries[j+1].Delta <= removalThreshold {
				// removable from sketch
				s.Entries[j+1].G += s.Entries[j].G
			} else {
				merged = append(merged, s.Entries[j])
			}
			j++
		} else if j == len(s.Entries) {
			// done with sketch; now only considering incoming
			if i+1 < len(incomingEntries) &&
				incomingEntries[i].G+incomingEntries[i+1].G+incomingEntries[i+1].Delta <= removalThreshold {
				// removable from incoming
				incomingEntries[i+1].G += incomingEntries[i].G
			} else {
				merged = append(merged, incomingEntries[i])
			}
			i++
		} else if incomingEntries[i].V < s.Entries[j].V {
			if incomingEntries[i].G+s.Entries[j].G+s.Entries[j].Delta <= removalThreshold {
				// removable from incoming
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
	s.incoming = make([]float64, 0, int(1/EPSILON))
}
