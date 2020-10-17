// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package stats

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/trace/exportable/stats/quantile"
)

// Hardcoded measures names for ease of reference
const (
	HITS     string = "hits"
	ERRORS          = "errors"
	DURATION        = "duration"
)

var (
	// DefaultCounts is an array of the measures we represent as Count by default
	DefaultCounts = [...]string{HITS, ERRORS, DURATION}
	// DefaultDistributions is an array of the measures we represent as Distribution by default
	// Not really used right now as we don't have a way to easily add new distros
	DefaultDistributions = [...]string{DURATION}
)

// Count represents one specific "metric" we track for a given tagset
// A count keeps track of the total for a metric during a given time in a certain dimension.
// By default we keep count of "hits", "errors" and "durations". Others can be added
// (from the Metrics map in a span), but they have to be enabled manually.
//
// Example: hits between X and X+5s for service:dogweb and resource:dash.list
type Count struct {
	Key     string `json:"key"`
	Name    string `json:"name"`    // the name of the trace/spans we count (was a member of TagSet)
	Measure string `json:"measure"` // represents the entity we count, e.g. "hits", "errors", "time" (was Name)
	TagSet  TagSet `json:"tagset"`  // set of tags for which we account this Distribution

	TopLevel float64 `json:"top_level"` // number of top-level spans contributing to this count

	Value float64 `json:"value"` // accumulated values
}

// Distribution represents a true image of the spectrum of values, allowing arbitrary quantile queries
// A distribution works the same way Counts do, but instead of accumulating values it keeps a sense of
// the repartition of the values. It uses the Greenwald-Khanna online summary algorithm.
//
// A distribution can answer to an arbitrary quantile query within a given epsilon. For each "range" of
// values in our pseudo-histogram we keep a trace ID (a sample) so that we can give the user an example
// of a trace for a given quantile query.
type Distribution struct {
	Key     string `json:"key"`
	Name    string `json:"name"`    // the name of the trace/spans we count (was a member of TagSet)
	Measure string `json:"measure"` // represents the entity we count, e.g. "hits", "errors", "time"
	TagSet  TagSet `json:"tagset"`  // set of tags for which we account this Distribution

	TopLevel float64 `json:"top_level"` // number of top-level spans contributing to this count

	Summary *quantile.SliceSummary `json:"summary"` // actual representation of data
}

// GrainKey generates the key used to aggregate counts and distributions
// which is of the form: name|measure|aggr
// for example: serve|duration|service:webserver
func GrainKey(name, measure, aggr string) string {
	return name + "|" + measure + "|" + aggr
}

// NewCount returns a new Count for a metric and a given tag set
func NewCount(m, ckey, name string, tgs TagSet) Count {
	return Count{
		Key:     ckey,
		Name:    name,
		Measure: m,
		TagSet:  tgs, // note: by doing this, tgs is a ref shared by all objects created with the same arg
		Value:   0.0,
	}
}

// Add adds some values to one count
func (c Count) Add(v float64) Count {
	c.Value += v
	return c
}

// Merge is used when 2 Counts represent the same thing and adds Values
func (c Count) Merge(c2 Count) Count {
	if c.Key != c2.Key {
		err := fmt.Errorf("Trying to merge non-homogoneous counts [%s] and [%s]", c.Key, c2.Key)
		panic(err)
	}

	c.Value += c2.Value
	return c
}

// NewDistribution returns a new Distribution for a metric and a given tag set
func NewDistribution(m, ckey, name string, tgs TagSet) Distribution {
	return Distribution{
		Key:     ckey,
		Name:    name,
		Measure: m,
		TagSet:  tgs, // note: by doing this, tgs is a ref shared by all objects created with the same arg
		Summary: quantile.NewSliceSummary(),
	}
}

// Add inserts the proper values in a given distribution from a span
func (d Distribution) Add(v float64, sampleID uint64) {
	d.Summary.Insert(v, sampleID)
}

// Merge is used when 2 Distributions represent the same thing and it merges the 2 underlying summaries
func (d Distribution) Merge(d2 Distribution) {
	// We don't check tagsets for distributions as we reaggregate without reallocating new structs
	d.Summary.Merge(d2.Summary)
}

// Weigh applies a weight factor to a distribution and return the result as a
// new distribution.
func (d Distribution) Weigh(weight float64) Distribution {
	d2 := Distribution(d)
	d2.Summary = quantile.WeighSummary(d.Summary, weight)
	return d2
}

// Copy returns a distro with the same data but a different underlying summary
func (d Distribution) Copy() Distribution {
	d2 := Distribution(d)
	d2.Summary = d.Summary.Copy()
	return d2
}

// Bucket is a time bucket to track statistic around multiple Counts
type Bucket struct {
	Start    int64 // Timestamp of start in our format
	Duration int64 // Duration of a bucket in nanoseconds

	// Stats indexed by keys
	Counts           map[string]Count        // All the counts
	Distributions    map[string]Distribution // All the distributions (e.g.: for quantile queries)
	ErrDistributions map[string]Distribution // All the error distributions (e.g.: for apdex, as they account for frustrated)
}

// NewBucket opens a new bucket for time ts and initializes it properly
func NewBucket(ts, d int64) Bucket {
	// The only non-initialized value is the Duration which should be set by whoever closes that bucket
	return Bucket{
		Start:            ts,
		Duration:         d,
		Counts:           make(map[string]Count),
		Distributions:    make(map[string]Distribution),
		ErrDistributions: make(map[string]Distribution),
	}
}

// IsEmpty just says if this stats bucket has no information (in which case it's useless)
func (sb Bucket) IsEmpty() bool {
	return len(sb.Counts) == 0 && len(sb.Distributions) == 0 && len(sb.ErrDistributions) == 0
}
