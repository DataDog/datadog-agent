// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package collector

import (
	"sort"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// FIXTURE
type TestCheck struct {
	uniqueID check.ID
	name     string
	stop     chan bool
}

func (c *TestCheck) Stop()                                                { c.stop <- true }
func (c *TestCheck) Configure(a, b integration.Data, source string) error { return nil }
func (c *TestCheck) Interval() time.Duration                              { return 1 * time.Minute }
func (c *TestCheck) Run() error                                           { <-c.stop; return nil }
func (c *TestCheck) GetWarnings() []error                                 { return []error{} }
func (c *TestCheck) GetMetricStats() (map[string]int64, error)            { return make(map[string]int64), nil }
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

func (c *TestCheck) Version() string {
	return ""
}

func (c *TestCheck) ConfigSource() string {
	return ""
}

func NewCheck() *TestCheck { return &TestCheck{stop: make(chan bool)} }
func NewCheckUnique(id check.ID, name string) *TestCheck {
	return &TestCheck{uniqueID: id, name: name, stop: make(chan bool)}
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
}

func (suite *CollectorTestSuite) TearDownTest() {
	suite.c.Stop()
	suite.c = nil
}

func (suite *CollectorTestSuite) TestNewCollector() {
	assert.NotNil(suite.T(), suite.c.runner)
	assert.NotNil(suite.T(), suite.c.scheduler)
	assert.Equal(suite.T(), started, suite.c.state)
}

func (suite *CollectorTestSuite) TestStop() {
	suite.c.Stop()
	assert.Nil(suite.T(), suite.c.runner)
	assert.Nil(suite.T(), suite.c.scheduler)
	assert.Equal(suite.T(), stopped, suite.c.state)
}

func (suite *CollectorTestSuite) TestRunCheck() {
	ch := NewCheck()

	// schedule a check
	id, err := suite.c.RunCheck(ch)
	assert.NotNil(suite.T(), id)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), 1, len(suite.c.checks))
	assert.Equal(suite.T(), ch, suite.c.checks["TestCheck"])

	// schedule the same check twice
	_, err = suite.c.RunCheck(ch)
	assert.NotNil(suite.T(), err)
	assert.Equal(suite.T(), "a check with ID TestCheck is already running", err.Error())
}

func (suite *CollectorTestSuite) TestReloadCheck() {
	ch := NewCheck()
	empty := integration.Data{}

	// schedule a check
	_, err := suite.c.RunCheck(ch)

	// check doesn't exist
	err = suite.c.ReloadCheck("foo", empty, empty, "test")
	assert.NotNil(suite.T(), err)
	assert.Equal(suite.T(), "cannot find a check with ID foo", err.Error())

	// all good
	err = suite.c.ReloadCheck("TestCheck", empty, empty, "test")
	assert.Nil(suite.T(), err)
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
}

func (suite *CollectorTestSuite) TestFind() {
	assert.False(suite.T(), suite.c.find("bar"))
	suite.c.checks["bar"] = nil
	assert.False(suite.T(), suite.c.find("foo"))
	assert.True(suite.T(), suite.c.find("bar"))
}

func (suite *CollectorTestSuite) TestDelete() {
	// delete a key that doesn't exist should be a noop
	assert.NotNil(suite.T(), suite.c)
	suite.c.delete("foo")

	// for good
	suite.c.checks["bar"] = nil
	assert.True(suite.T(), suite.c.find("bar"))
	suite.c.delete("bar")
	assert.False(suite.T(), suite.c.find("bar"))
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

	assert.False(suite.T(), suite.c.find("foo"))
	assert.False(suite.T(), suite.c.find("bar"))
	assert.True(suite.T(), suite.c.find("baz"))
	assert.True(suite.T(), suite.c.find("qux"))

	// Reload check: kill 2 & start no new instances
	killed, err = suite.c.ReloadAllCheckInstances("TestCheck", []check.Check{})
	assert.Nil(suite.T(), err)
	sort.Sort(ChecksList(killed))
	assert.Equal(suite.T(), killed, []check.ID{"baz", "qux"})

	assert.Zero(suite.T(), len(suite.c.checks))
}

func TestCollectorSuite(t *testing.T) {
	suite.Run(t, new(CollectorTestSuite))
}
