// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStarter(t *testing.T) {
	ch, components := newComponents(3)
	starter := NewStarter(components[0], components[1])
	starter.Add(components[2])
	starter.Start()
	require.Equal(t, "start c0", <-ch)
	require.Equal(t, "start c1", <-ch)
	require.Equal(t, "start c2", <-ch)
}
