// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSerialStopper(t *testing.T) {
	ch, components := newComponents(3)
	stopper := NewSerialStopper(components[0], components[1])
	stopper.Add(components[2])
	stopper.Stop()
	require.Equal(t, "stop c0", <-ch)
	require.Equal(t, "stop c1", <-ch)
	require.Equal(t, "stop c2", <-ch)
}
