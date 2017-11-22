// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package mocksender

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type unittestMock struct {
	mock.Mock
}

// Test the behavior of AnythingBut
// In this method we could observe an intend failure of "AssertNotCalled" tied to the localTester
// Method returns the failure state of the localTester
func (u *unittestMock) assertAnythingBut(t *testing.T, s string) bool {
	u.Mock.AssertCalled(t, "dummyMethod", mock.AnythingOfType("string"))

	// Create a local testing.T just for the following AssertNotCalled
	localTester := &testing.T{}
	// Pass the [localTester *testing.T] instead of [t *testing.T]
	u.Mock.AssertNotCalled(localTester, "dummyMethod", AnythingBut(s))
	return localTester.Failed()
}

// A dummy Method to be called
func (u *unittestMock) dummyMethod(s string) {
	u.Called(s)
}

func TestAnythingBut(t *testing.T) {
	m := &unittestMock{}
	m.On("dummyMethod", mock.AnythingOfType("string")).Return()
	m.dummyMethod("ok")

	result := m.assertAnythingBut(t, "ok")
	assert.False(t, result)

	m.dummyMethod("ko")
	result = m.assertAnythingBut(t, "ok")
	assert.True(t, result)
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
	sender.ServiceCheck("docker.exit", metrics.ServiceCheckOK, "", tags, message)
	sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckOK, "", tags, message)

	tags = append(tags, "a", "b", "c")
	message = "message 2"
	sender.ServiceCheck("docker.exit", metrics.ServiceCheckCritical, "", tags, message)
	sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckCritical, "", tags, message)

	message = "message 3"
	tags = []string{"1", "2"}
	sender.ServiceCheck("docker.exit", metrics.ServiceCheckWarning, "", tags, message)
	sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckWarning, "", tags, message)

	message = "message 4"
	tags = append(tags, "container_name:redis")
	sender.ServiceCheck("docker.exit", metrics.ServiceCheckWarning, "", tags, message)
	sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckWarning, "", tags, message)
}

func TestMockedEvent(t *testing.T) {
	sender := NewMockSender("2")
	sender.SetupAcceptAll()

	tags := []string{"one", "two"}

	eventTimestamp := time.Date(2010, 01, 01, 01, 01, 01, 00, time.UTC).Unix()
	eventOne := metrics.Event{
		Ts:             eventTimestamp,
		EventType:      "docker",
		Tags:           tags,
		AggregationKey: "docker:busybox",
		SourceTypeName: "docker",
		Priority:       metrics.EventPriorityNormal,
	}
	sender.Event(eventOne)
	sender.AssertEvent(t, eventOne, time.Second)

	eventTwo := metrics.Event{
		Ts:             eventTimestamp,
		EventType:      "docker",
		Tags:           tags,
		AggregationKey: "docker:redis",
		SourceTypeName: "docker",
		Priority:       metrics.EventPriorityNormal,
	}
	sender.AssertEventMissing(t, eventTwo, 0)

	sender.Event(eventTwo)
	sender.AssertEvent(t, eventTwo, 0)

	eventTwo.Ts = eventTwo.Ts + 10
	sender.AssertEventMissing(t, eventTwo, 0)

	allowedDelta := time.Since(time.Unix(eventTimestamp, 0))
	sender.AssertEvent(t, eventTwo, allowedDelta)
}
