// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParallelStopper(t *testing.T) {
	ch, components := newComponents(3)
	stopper := NewParallelStopper(components[0], components[1])
	stopper.Add(components[2])
	stopper.Stop()
	close(ch)

	var events []string
	for e := range ch {
		events = append(events, e)
	}

	sort.Strings(events)
	require.Equal(t, []string{"stop c0", "stop c1", "stop c2"}, events)
}
