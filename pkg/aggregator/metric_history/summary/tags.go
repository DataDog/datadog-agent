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
	// TODO: implement
	return TagPartition{
		ConstantTags: make(map[string]string),
		VaryingTags:  make(map[string][]string),
	}
}
