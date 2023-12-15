// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package mocksender

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

type unittestMock struct {
	mock.Mock
}

// A dummy Method to be called
func (u *unittestMock) dummyMethod(s interface{}) {
	u.Called(s)
}

// In this method we could observe an intend failure of "AssertNotCalled" tied to the localTester
func TestAnythingBut(t *testing.T) {
	m := &unittestMock{}
	m.On("dummyMethod", mock.AnythingOfType("string")).Return()

	m.dummyMethod("ok")
	m.AssertCalled(t, "dummyMethod", mock.AnythingOfType("string"))

	m.dummyMethod("ko")
	m.AssertCalled(t, "dummyMethod", mock.AnythingOfType("string"))

	// Create a local testing.T just for the following AssertNotCalled
	localTester := &testing.T{}
	// Pass the [localTester *testing.T] instead of [t *testing.T]
	m.AssertNotCalled(localTester, "dummyMethod", AnythingBut("ok"))
	// Expected a failure on localTester
	assert.True(t, localTester.Failed())
}

// In this method we could observe an intend failure of "AssertNotCalled" tied to the localTester
func TestIsGreaterOrEqual(t *testing.T) {
	m := &unittestMock{}
	m.On("dummyMethod", mock.AnythingOfType("float64")).Return()

	const dummyValue = 3.0
	m.dummyMethod(dummyValue)

	for i := 0.0; i < dummyValue+1; i++ {
		m.AssertCalled(t, "dummyMethod", IsGreaterOrEqual(i))
	}
	m.AssertNotCalled(t, "dummyMethod", IsGreaterOrEqual(dummyValue+1))

	// Create a local testing.T just for the following AssertCalled
	localTester := &testing.T{}
	// Pass the [localTester *testing.T] instead of [t *testing.T]
	m.AssertCalled(localTester, "dummyMethod", IsGreaterOrEqual(dummyValue+1))
	// Expected a failure on localTester
	assert.True(t, localTester.Failed())
}

func TestExpectedInActual(t *testing.T) {
	assert.True(t, expectedInActual([]string{}, []string{}))
	assert.True(t, expectedInActual([]string{}, []string{"one"}))
	assert.True(t, expectedInActual([]string{"one"}, []string{"one"}))
	assert.True(t, expectedInActual([]string{}, []string{"one", "two"}))
	assert.True(t, expectedInActual([]string{"one"}, []string{"one", "two"}))
	assert.True(t, expectedInActual([]string{"one", "two"}, []string{"one", "two"}))

	array := []string{"one", "two", "three"}
	assert.True(t, expectedInActual(array[0:0], array))
	assert.True(t, expectedInActual(array[:1], array))
	assert.True(t, expectedInActual(array[:1], array[:1]))

	assert.False(t, expectedInActual(array, []string{"one", "two"}))
	assert.False(t, expectedInActual(array, []string{"one", "two", "four"}))
	assert.False(t, expectedInActual(array, []string{}))
	assert.False(t, expectedInActual([]string{"one"}, []string{}))

	assert.False(t, expectedInActual(array, array[:1]))
	assert.False(t, expectedInActual(array[:1], []string{}))
	assert.False(t, expectedInActual([]string{"one", "two", "three"}, array[0:0]))
}

func TestMockedServiceCheck(t *testing.T) {
	sender := NewMockSender("1")
	sender.SetupAcceptAll()

	tags := []string{"one", "two"}
	message := "message 1"
	sender.ServiceCheck("docker.exit", servicecheck.ServiceCheckOK, "", tags, message)
	sender.AssertServiceCheck(t, "docker.exit", servicecheck.ServiceCheckOK, "", tags, message)

	tags = append(tags, "a", "b", "c")
	message = "message 2"
	sender.ServiceCheck("docker.exit", servicecheck.ServiceCheckCritical, "", tags, message)
	sender.AssertServiceCheck(t, "docker.exit", servicecheck.ServiceCheckCritical, "", tags, message)

	message = "message 3"
	tags = []string{"1", "2"}
	sender.ServiceCheck("docker.exit", servicecheck.ServiceCheckWarning, "", tags, message)
	sender.AssertServiceCheck(t, "docker.exit", servicecheck.ServiceCheckWarning, "", tags, message)

	message = "message 4"
	tags = append(tags, "container_name:redis")
	sender.ServiceCheck("docker.exit", servicecheck.ServiceCheckWarning, "", tags, message)
	sender.AssertServiceCheck(t, "docker.exit", servicecheck.ServiceCheckWarning, "", tags, message)
}

func TestMockedEvent(t *testing.T) {
	sender := NewMockSender("2")
	sender.SetupAcceptAll()

	tags := []string{"one", "two"}

	eventTimestamp := time.Date(2010, 01, 01, 01, 01, 01, 00, time.UTC).Unix()
	eventOne := event.Event{
		Ts:             eventTimestamp,
		EventType:      "docker",
		Tags:           tags,
		AggregationKey: "docker:busybox",
		SourceTypeName: "docker",
		Priority:       event.EventPriorityNormal,
	}
	sender.Event(eventOne)
	sender.AssertEvent(t, eventOne, time.Second)

	eventTwo := event.Event{
		Ts:             eventTimestamp,
		EventType:      "docker",
		Tags:           tags,
		AggregationKey: "docker:redis",
		SourceTypeName: "docker",
		Priority:       event.EventPriorityNormal,
	}
	sender.AssertEventMissing(t, eventTwo, 0)

	sender.Event(eventTwo)
	sender.AssertEvent(t, eventTwo, 0)

	eventTwo.Ts = eventTwo.Ts + 10
	sender.AssertEventMissing(t, eventTwo, 0)

	allowedDelta := time.Since(time.Unix(eventTimestamp, 0))
	sender.AssertEvent(t, eventTwo, allowedDelta)
}
