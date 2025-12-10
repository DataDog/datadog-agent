// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

// PartitionTags analyzes events and separates tags into constant vs varying.
// A tag is "constant" if all events have the same value for that key.
// A tag is "varying" if events have different values for that key.
// Tags that only appear on some events are treated as varying.
func PartitionTags(events []AnomalyEvent) TagPartition {
	partition := TagPartition{
		ConstantTags: make(map[string]string),
		VaryingTags:  make(map[string][]string),
	}

	// Edge case: no events or empty events
	if len(events) == 0 {
		return partition
	}

	// Edge case: single event - all tags are constant (nothing to vary against)
	if len(events) == 1 {
		if events[0].Tags != nil {
			for k, v := range events[0].Tags {
				partition.ConstantTags[k] = v
			}
		}
		return partition
	}

	// Collect all unique tag keys across all events
	allKeys := make(map[string]bool)
	for _, event := range events {
		if event.Tags == nil {
			continue
		}
		for k := range event.Tags {
			allKeys[k] = true
		}
	}

	// For each tag key, determine if it's constant or varying
	for key := range allKeys {
		// Collect all values for this key, tracking which events have it
		values := make(map[string]bool)
		presentCount := 0

		for _, event := range events {
			if event.Tags == nil {
				continue
			}
			if val, exists := event.Tags[key]; exists {
				values[val] = true
				presentCount++
			}
		}

		// If the key is not present on all events, it's varying
		if presentCount != len(events) {
			// Collect distinct values
			distinctValues := make([]string, 0, len(values))
			for v := range values {
				distinctValues = append(distinctValues, v)
			}
			partition.VaryingTags[key] = distinctValues
			continue
		}

		// If there's more than one distinct value, it's varying
		if len(values) > 1 {
			// Collect distinct values
			distinctValues := make([]string, 0, len(values))
			for v := range values {
				distinctValues = append(distinctValues, v)
			}
			partition.VaryingTags[key] = distinctValues
		} else {
			// Exactly one value across all events - it's constant
			for v := range values {
				partition.ConstantTags[key] = v
				break
			}
		}
	}

	return partition
}
