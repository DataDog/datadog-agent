// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package collector

import (
	"sort"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/internal/middleware"
)

// FIXTURE
type TestCheck struct {
	check.StubCheck
	mock.Mock
	uniqueID check.ID
	name     string
	stop     chan bool
}

func (c *TestCheck) Stop()                   { c.stop <- true }
func (c *TestCheck) Cancel()                 { c.Called() }
func (c *TestCheck) Interval() time.Duration { return 1 * time.Minute }
func (c *TestCheck) Run() error              { <-c.stop; return nil }
func (c *TestCheck) ID() check.ID {
	if c.uniqueID != "" {
		return c.uniqueID
	}
	return check.ID(c.String())
}
func (c *TestCheck) String() string {
	if c.name != "" {
		return c.name
	}
	return "TestCheck"
}

func NewCheck() *TestCheck {
	c := &TestCheck{
		stop: make(chan bool),
	}
	c.On("Cancel").Maybe()
	return c
}

func NewCheckUnique(id check.ID, name string) *TestCheck {
	c := NewCheck()
	c.uniqueID = id
	c.name = name
	return c
}

func NewCheckSlowCancel(after time.Duration) *TestCheck {
	c := &TestCheck{
		stop: make(chan bool),
	}
	c.On("Cancel").After(after)
	return c
}

// ChecksList is a sort.Interface so we can use the Sort function
type ChecksList []check.ID

func (p ChecksList) Len() int           { return len(p) }
func (p ChecksList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p ChecksList) Less(i, j int) bool { return p[i] < p[j] }

type CollectorTestSuite struct {
	suite.Suite
	c *Collector
}

func (suite *CollectorTestSuite) SetupTest() {
	suite.c = NewCollector()
	suite.c.Start()
}

func (suite *CollectorTestSuite) TearDownTest() {
	suite.c.Stop()
	suite.c = nil
}

func (suite *CollectorTestSuite) TestNewCollector() {
	assert.NotNil(suite.T(), suite.c.runner)
	assert.NotNil(suite.T(), suite.c.scheduler)
	assert.Equal(suite.T(), started, suite.c.state.Load())
}

func (suite *CollectorTestSuite) TestStop() {
	suite.c.Stop()
	assert.Nil(suite.T(), suite.c.runner)
	assert.Nil(suite.T(), suite.c.scheduler)
	assert.Equal(suite.T(), stopped, suite.c.state.Load())
}

func (suite *CollectorTestSuite) TestRunCheck() {
	ch := NewCheck()

	// schedule a check
	id, err := suite.c.RunCheck(ch)
	assert.NotNil(suite.T(), id)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), 1, len(suite.c.checks))
	assert.Equal(suite.T(), ch, suite.c.checks["TestCheck"].Inner())

	// schedule the same check twice
	_, err = suite.c.RunCheck(ch)
	assert.NotNil(suite.T(), err)
	assert.Equal(suite.T(), "a check with ID TestCheck is already running", err.Error())
}

func (suite *CollectorTestSuite) TestStopCheck() {
	ch := NewCheck()

	// schedule a check
	_, err := suite.c.RunCheck(ch)
	assert.Nil(suite.T(), err)

	// all good
	err = suite.c.StopCheck("TestCheck")
	assert.Nil(suite.T(), err)
	assert.Zero(suite.T(), len(suite.c.checks))
	ch.AssertNumberOfCalls(suite.T(), "Cancel", 1)
}

func (suite *CollectorTestSuite) TestCancelCheck_TimeoutIsApplied() {
	ch := NewCheckSlowCancel(10 * time.Second)

	start := time.Now()
	err := suite.c.cancelCheck(ch, 100*time.Millisecond)
	assert.NotNil(suite.T(), err)
	assert.WithinDuration(suite.T(), start, time.Now(), 5*time.Second)
	// assert that `Cancel` was actually called on the check, which may be flaky if the goroutine
	// that calls `Cancel` didn't have a chance to be scheduled before the timeout is hit.
	ch.AssertNumberOfCalls(suite.T(), "Cancel", 1)
}

func (suite *CollectorTestSuite) TestGet() {
	_, found := suite.c.get("bar")
	assert.False(suite.T(), found)

	suite.c.checks["bar"] = middleware.NewCheckWrapper(NewCheck())
	_, found = suite.c.get("foo")
	assert.False(suite.T(), found)
	c, found := suite.c.get("bar")
	assert.True(suite.T(), found)
	assert.Equal(suite.T(), suite.c.checks["bar"], c)
}

func (suite *CollectorTestSuite) TestDelete() {
	// delete a key that doesn't exist should be a noop
	assert.NotNil(suite.T(), suite.c)
	suite.c.delete("foo")

	// for good
	suite.c.checks["bar"] = nil
	_, found := suite.c.get("bar")
	assert.True(suite.T(), found)
	suite.c.delete("bar")
	_, found = suite.c.get("bar")
	assert.False(suite.T(), found)
}

func (suite *CollectorTestSuite) TestStarted() {
	assert.True(suite.T(), suite.c.started())
	suite.c.Stop()
	assert.False(suite.T(), suite.c.started())
}

func (suite *CollectorTestSuite) TestGetAllInstanceIDs() {
	// Schedule 2 instances of TestCheck1 and 1 instance of TestCheck2
	ch1 := NewCheckUnique("foo", "TestCheck1")
	ch2 := NewCheckUnique("bar", "TestCheck1")
	ch3 := NewCheckUnique("baz", "TestCheck2")
	id1, err := suite.c.RunCheck(ch1)
	assert.NotNil(suite.T(), id1)
	assert.Nil(suite.T(), err)
	id2, err := suite.c.RunCheck(ch2)
	assert.NotNil(suite.T(), id2)
	assert.Nil(suite.T(), err)
	id3, err := suite.c.RunCheck(ch3)
	assert.NotNil(suite.T(), id3)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), 3, len(suite.c.checks))

	ids := suite.c.GetAllInstanceIDs("TestCheck1")
	assert.Equal(suite.T(), 2, len(ids))
	sort.Sort(ChecksList(ids))
	expected := []check.ID{"bar", "foo"}
	for i := range expected {
		assert.Equal(suite.T(), ids[i], expected[i])
	}
}

func (suite *CollectorTestSuite) TestReloadAllCheckInstances() {
	// Schedule 2 check instances
	ch1 := NewCheckUnique("foo", "TestCheck")
	ch2 := NewCheckUnique("bar", "TestCheck")
	id1, err := suite.c.RunCheck(ch1)
	assert.NotNil(suite.T(), id1)
	assert.Nil(suite.T(), err)
	id2, err := suite.c.RunCheck(ch2)
	assert.NotNil(suite.T(), id2)
	assert.Nil(suite.T(), err)

	// Reload check: kill 2 & start 2 new instances
	ch3 := NewCheckUnique("baz", "TestCheck")
	ch4 := NewCheckUnique("qux", "TestCheck")
	killed, err := suite.c.ReloadAllCheckInstances("TestCheck", []check.Check{ch3, ch4})
	assert.Nil(suite.T(), err)
	sort.Sort(ChecksList(killed))
	assert.Equal(suite.T(), killed, []check.ID{"bar", "foo"})

	_, found := suite.c.get("foo")
	assert.False(suite.T(), found)
	_, found = suite.c.get("bar")
	assert.False(suite.T(), found)
	_, found = suite.c.get("baz")
	assert.True(suite.T(), found)
	_, found = suite.c.get("qux")
	assert.True(suite.T(), found)

	// Reload check: kill 2 & start no new instances
	killed, err = suite.c.ReloadAllCheckInstances("TestCheck", []check.Check{})
	assert.Nil(suite.T(), err)
	sort.Sort(ChecksList(killed))
	assert.Equal(suite.T(), killed, []check.ID{"baz", "qux"})

	assert.Zero(suite.T(), len(suite.c.checks))
}
