// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metrics

import (
	"errors"
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	jsoniter "github.com/json-iterator/go"
)

// GroupedSeries represents a list of Serie grouped by metric name
type GroupedSeries [][]*Serie

// SortAndGroupSeries groups a slice of series per metric name
func SortAndGroupSeries(in Series) GroupedSeries {
	if len(in) < 2 {
		return GroupedSeries{in}
	}

	sort.Slice(in, func(i, j int) bool { return in[i].Name < in[j].Name })

	var out GroupedSeries
	groupName := in[0].Name
	groupStart := 0

	for i := 1; i < len(in); i++ {
		if in[i].Name != groupName {
			// Close previous slice
			out = append(out, in[groupStart:i])

			// Open new slice
			groupName = in[i].Name
			groupStart = i
		}
	}

	// Close last slice
	out = append(out, in[groupStart:])

	return out
}

// JSONHeader prints the payload header for this type
func (series GroupedSeries) JSONHeader() []byte {
	return []byte(`{"series":[`)
}

// Len returns the number of items to marshal
func (series GroupedSeries) Len() int {
	return len(series)
}

// JSONItem prints the json representation of an item
func (series GroupedSeries) JSONItem(i int) ([]byte, error) {
	if i < 0 || i > len(series)-1 {
		return nil, errors.New("out of range")
	}

	group := series[i]

	var bufferSize int
	if len(group) > 64 {
		// Pre-allocate an appropriate buffer size
		// assuming the item size will be similar
		b, _ := marshaller.Marshal(group[0])
		bufferSize = len(b) * len(group)
	}

	var needComa bool
	jsonOut := jsoniter.NewStream(marshaller, nil, bufferSize)
	for _, serie := range group {
		if needComa {
			jsonOut.Write(jsonSeparator)
		} else {
			needComa = true
		}
		populateDeviceField(serie)
		jsonOut.WriteVal(serie)
	}

	return jsonOut.Buffer(), nil
}

// JSONFooter prints the payload footer for this type
func (series GroupedSeries) JSONFooter() []byte {
	return []byte(`]}`)
}

// DescribeItem returns a text description for logs
func (series GroupedSeries) DescribeItem(i int) string {
	if i < 0 || i > len(series)-1 {
		return "out of range"
	}
	if len(series[i]) == 0 {
		return "empty item"
	}
	return fmt.Sprintf("name %q, %d series", series[i][0].Name, len(series[i]))
}

// MarshalJSON is not implemented for GroupedSeries
func (series GroupedSeries) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

// Marshal is not implemented for GroupedSeries
func (series GroupedSeries) Marshal() ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

// SplitPayload is not implemented for GroupedSeries
func (series GroupedSeries) SplitPayload(int) ([]marshaler.Marshaler, error) {
	return nil, fmt.Errorf("not implemented")
}
