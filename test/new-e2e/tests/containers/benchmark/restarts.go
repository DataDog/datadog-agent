// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// CompareRestarts returns an error listing the containers that restarted between two records
// of the restart counts of containers, keyed by "<pod>/<container>", and those of the pods
// replaced in between.
func CompareRestarts(before, after map[string]int32) error {
	var changes []string
	for _, container := range slices.Sorted(maps.Keys(before)) {
		count, found := after[container]
		switch {
		case !found:
			changes = append(changes, container+": pod replaced")
		case count > before[container]:
			changes = append(changes, fmt.Sprintf("%s %d→%d", container, before[container], count))
		}
	}
	for _, container := range slices.Sorted(maps.Keys(after)) {
		if _, found := before[container]; !found {
			changes = append(changes, container+": new pod")
		}
	}
	if len(changes) > 0 {
		return errors.New(strings.Join(changes, ", "))
	}
	return nil
}
