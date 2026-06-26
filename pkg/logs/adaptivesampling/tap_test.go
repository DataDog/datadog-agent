// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package adaptivesampling

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureTokenizedLogObserver struct {
	events []TokenizedLogEvent
}

func (o *captureTokenizedLogObserver) ObserveTokenizedLog(event TokenizedLogEvent) {
	o.events = append(o.events, event)
}

func TestTokenizedLogObserver(t *testing.T) {
	assert.False(t, HasTokenizedLogObserver())

	observer := &captureTokenizedLogObserver{}
	cleanup := SetTokenizedLogObserver(observer)
	t.Cleanup(cleanup)

	require.True(t, HasTokenizedLogObserver())
	ObserveTokenizedLog(TokenizedLogEvent{ContainerID: "container-a", PatternHash: "abc"})

	require.Len(t, observer.events, 1)
	assert.Equal(t, "container-a", observer.events[0].ContainerID)
	assert.Equal(t, "abc", observer.events[0].PatternHash)
}
