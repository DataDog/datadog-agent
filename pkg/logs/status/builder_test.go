// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceAreGroupedByIntegrations(t *testing.T) {
	sourcesToTrack := []*SourceToTrack{
		NewSourceToTrack("foo", NewTracker("")),
		NewSourceToTrack("bar", NewTracker("")),
		NewSourceToTrack("foo", NewTracker("")),
	}
	builder := NewBuilder(sourcesToTrack)

	var integration Integration

	status := builder.Build()
	assert.Equal(t, true, status.IsRunning)
	assert.Equal(t, 2, len(status.Integrations))

	integration = status.Integrations[0]
	assert.Equal(t, "foo", integration.Name)
	assert.Equal(t, 2, len(integration.Sources))

	integration = status.Integrations[1]
	assert.Equal(t, "bar", integration.Name)
	assert.Equal(t, 1, len(integration.Sources))
}
