// +build !cpython

package collector

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// FIXTURE
type TestCheck struct {
	stop chan bool
}

func (c *TestCheck) String() string                        { return "TestCheck" }
func (c *TestCheck) Stop()                                 { c.stop <- true }
func (c *TestCheck) Configure(a, b check.ConfigData) error { return nil }
func (c *TestCheck) InitSender()                           {}
func (c *TestCheck) Interval() time.Duration               { return 1 * time.Minute }
func (c *TestCheck) Run() error                            { <-c.stop; return nil }
func (c *TestCheck) ID() check.ID                          { return check.ID(c.String()) }

func NewCheck() *TestCheck { return &TestCheck{stop: make(chan bool)} }

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
	err := suite.c.RunCheck(ch)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), 1, len(suite.c.checks))
	assert.Equal(suite.T(), ch, suite.c.checks["TestCheck"])

	// schedule the same check twice
	err = suite.c.RunCheck(ch)
	assert.NotNil(suite.T(), err)
	assert.Equal(suite.T(), "a check with ID TestCheck is already running", err.Error())
}

func (suite *CollectorTestSuite) TestReloadCheck() {
	ch := NewCheck()
	empty := check.ConfigData{}

	// schedule a check
	err := suite.c.RunCheck(ch)

	// check doesn't exist
	err = suite.c.ReloadCheck("foo", empty, empty)
	assert.NotNil(suite.T(), err)
	assert.Equal(suite.T(), "cannot find a check with ID foo", err.Error())

	// all good
	err = suite.c.ReloadCheck("TestCheck", empty, empty)
	assert.Nil(suite.T(), err)
}

func (suite *CollectorTestSuite) TestStopCheck() {
	ch := NewCheck()

	// schedule a check
	err := suite.c.RunCheck(ch)

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

func TestCollectorSuite(t *testing.T) {
	suite.Run(t, new(CollectorTestSuite))
}
