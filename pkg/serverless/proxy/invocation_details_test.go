// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build serverlessexperimental

package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsCompleteTrue(t *testing.T) {
	invokeHeaders := make(map[string][]string)
	invokeHeaders["a"] = []string{"aaa"}
	completeItem := &invocationDetails{
		startTime:          time.Now(),
		endTime:            time.Now(),
		isError:            false,
		invokeHeaders:      invokeHeaders,
		invokeEventPayload: "bla bla bla",
	}
	assert.True(t, completeItem.isComplete())
}

func TestIsCompleteFalse(t *testing.T) {
	invokeHeaders := make(map[string][]string)
	invokeHeaders["a"] = []string{"aaa"}

	// startTime is missing
	inCompleteItem := &invocationDetails{
		endTime:            time.Now(),
		isError:            false,
		invokeHeaders:      invokeHeaders,
		invokeEventPayload: "bla bla bla",
	}
	assert.False(t, inCompleteItem.isComplete())

	// endTime is missing
	inCompleteItem = &invocationDetails{
		startTime:          time.Now(),
		isError:            false,
		invokeHeaders:      invokeHeaders,
		invokeEventPayload: "bla bla bla",
	}
	assert.False(t, inCompleteItem.isComplete())

	// invokeHeaders is missing
	inCompleteItem = &invocationDetails{
		startTime:          time.Now(),
		endTime:            time.Now(),
		isError:            false,
		invokeEventPayload: "bla bla bla",
	}
	assert.False(t, inCompleteItem.isComplete())

	// invokePayload is missing
	inCompleteItem = &invocationDetails{
		startTime:     time.Now(),
		endTime:       time.Now(),
		isError:       false,
		invokeHeaders: invokeHeaders,
	}
	assert.False(t, inCompleteItem.isComplete())
}

func TestReset(t *testing.T) {
	invokeHeaders := make(map[string][]string)
	invokeHeaders["a"] = []string{"aaa"}
	item := &invocationDetails{
		startTime:          time.Now(),
		endTime:            time.Now(),
		isError:            false,
		invokeHeaders:      invokeHeaders,
		invokeEventPayload: "bla bla bla",
	}
	item.reset()
	assert.False(t, item.isComplete())
	assert.True(t, item.startTime.IsZero())
	assert.True(t, item.endTime.IsZero())
	assert.Nil(t, item.invokeHeaders)
	assert.True(t, len(item.invokeEventPayload) == 0)
}
