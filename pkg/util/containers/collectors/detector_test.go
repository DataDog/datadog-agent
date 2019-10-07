// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package collectors

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type MockCollector struct {
	mock.Mock
}

func (o *MockCollector) Detect() error {
	args := o.Called()
	return args.Error(0)
}

func (o *MockCollector) List() ([]*containers.Container, error) {
	args := o.Called()
	return nil, args.Error(0)
}

func (o *MockCollector) UpdateMetrics(list []*containers.Container) error {
	args := o.Called(list)
	return args.Error(0)
}

// registerMock creates and registers a new MockCollector in the catalog
func registerMock(name string, priority CollectorPriority) *MockCollector {
	c := &MockCollector{}
	factory := func() Collector { return c }
	registerCollector(name, factory, priority)
	return c
}

var willRetryError = &retry.Error{
	LogicError:    errors.New("temp error"),
	RessourceName: "retry",
	RetryStatus:   retry.FailWillRetry,
}
var permaFailRetryError = &retry.Error{
	LogicError:    errors.New("fatal error"),
	RessourceName: "retry",
	RetryStatus:   retry.PermaFail,
}

type DetectorTestSuite struct {
	suite.Suite
	originalCatalog    Catalog
	originalPriorities map[string]CollectorPriority
}

// SetupSuite saves the original catalog
func (suite *DetectorTestSuite) SetupSuite() {
	suite.originalCatalog = defaultCatalog
	suite.originalPriorities = collectorPriorities
	config.SetupLogger(
		config.LoggerName("test"),
		"debug",
		"",
		"",
		false,
		true,
		false,
	)
}

// TearDownSuite restores the original catalog
func (suite *DetectorTestSuite) TearDownSuite() {
	defaultCatalog = suite.originalCatalog
	collectorPriorities = suite.originalPriorities
}

// Empty the catalog before each test
func (suite *DetectorTestSuite) SetupTest() {
	defaultCatalog = make(Catalog)
	collectorPriorities = make(map[string]CollectorPriority)
}

// TestConfigureOneAvailable forces one valid collector
func (suite *DetectorTestSuite) TestConfigureOneAvailable() {
	one := registerMock("one", NodeRuntime)
	one.On("Detect").Return(nil).Once()
	two := registerMock("two", NodeRuntime)

	d := NewDetector("one")
	assert.Len(suite.T(), d.candidates, 1)
	assert.Len(suite.T(), d.detected, 0)

	c, n, err := d.GetPreferred()
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "one", n)
	assert.Equal(suite.T(), one, c)

	one.AssertNumberOfCalls(suite.T(), "Detect", 1)
	two.AssertNumberOfCalls(suite.T(), "Detect", 0)
	assert.Nil(suite.T(), d.candidates)
	assert.Nil(suite.T(), d.detected)
}

// TestConfigureOneUnknown forces one unknown collector
func (suite *DetectorTestSuite) TestConfigureOneUnknown() {
	two := registerMock("two", NodeRuntime)

	d := NewDetector("one")
	assert.Len(suite.T(), d.candidates, 0)
	assert.Len(suite.T(), d.detected, 0)

	c, n, err := d.GetPreferred()
	assert.Equal(suite.T(), ErrPermaFail, err)
	assert.Equal(suite.T(), "", n)
	assert.Equal(suite.T(), nil, c)

	two.AssertNumberOfCalls(suite.T(), "Detect", 0)
	assert.Nil(suite.T(), d.candidates)
	assert.Nil(suite.T(), d.detected)

	c, n, err = d.GetPreferred()
	assert.Equal(suite.T(), ErrPermaFail, err)
	assert.Equal(suite.T(), "", n)
	assert.Equal(suite.T(), nil, c)
}

// TestConfigureOneRetry makes sure we retry collectors,
// then select the new one based on higher priority
func (suite *DetectorTestSuite) TestConfigureRetry() {
	ok := registerMock("ok", NodeRuntime)
	ok.On("Detect").Return(nil)
	noRetry := registerMock("noretry", NodeRuntime)
	noRetry.On("Detect").Return(errors.New("retry not implemented"))
	fail := registerMock("fail", NodeOrchestrator)
	fail.On("Detect").Return(permaFailRetryError)
	retry := registerMock("retry", NodeOrchestrator)
	retry.On("Detect").Return(willRetryError).Once()
	retry.On("Detect").Return(nil).Once()

	d := NewDetector("")
	assert.Len(suite.T(), d.candidates, 4)
	assert.Len(suite.T(), d.detected, 0)

	// First run detects ok and keeps retry as candidate
	c, n, err := d.GetPreferred()
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "ok", n)
	assert.Equal(suite.T(), ok, c)
	assert.Len(suite.T(), d.candidates, 1)
	assert.Len(suite.T(), d.detected, 1)

	ok.AssertNumberOfCalls(suite.T(), "Detect", 1)
	noRetry.AssertNumberOfCalls(suite.T(), "Detect", 1)
	fail.AssertNumberOfCalls(suite.T(), "Detect", 1)
	retry.AssertNumberOfCalls(suite.T(), "Detect", 1)

	// Second run detects retry and uses this one instead
	c, n, err = d.GetPreferred()
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "retry", n)
	assert.Equal(suite.T(), retry, c)

	ok.AssertNumberOfCalls(suite.T(), "Detect", 1)
	noRetry.AssertNumberOfCalls(suite.T(), "Detect", 1)
	fail.AssertNumberOfCalls(suite.T(), "Detect", 1)
	retry.AssertNumberOfCalls(suite.T(), "Detect", 2)

	assert.Nil(suite.T(), d.candidates)
	assert.Nil(suite.T(), d.detected)

	// Third run should be a noop
	c, n, err = d.GetPreferred()
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "retry", n)
	assert.Equal(suite.T(), retry, c)

	ok.AssertNumberOfCalls(suite.T(), "Detect", 1)
	noRetry.AssertNumberOfCalls(suite.T(), "Detect", 1)
	fail.AssertNumberOfCalls(suite.T(), "Detect", 1)
	retry.AssertNumberOfCalls(suite.T(), "Detect", 2)
}

// TestConfigureTwoByName autodetects between two valid collectors,
// selected by alphabetic order
func (suite *DetectorTestSuite) TestConfigureTwoByName() {
	one := registerMock("one", NodeRuntime)
	one.On("Detect").Return(nil).Once()
	two := registerMock("two", NodeRuntime)
	two.On("Detect").Return(nil).Once()

	d := NewDetector("")
	assert.Len(suite.T(), d.candidates, 2)
	assert.Len(suite.T(), d.detected, 0)

	c, n, err := d.GetPreferred()
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "one", n)
	assert.Equal(suite.T(), one, c)

	one.AssertNumberOfCalls(suite.T(), "Detect", 1)
	two.AssertNumberOfCalls(suite.T(), "Detect", 1)
	assert.Nil(suite.T(), d.candidates)
	assert.Nil(suite.T(), d.detected)
}

// TestConfigureTwoByPrio autodetects between two valid collectors,
// selected by priority
func (suite *DetectorTestSuite) TestConfigureTwoByPrio() {
	one := registerMock("one", NodeRuntime)
	one.On("Detect").Return(nil).Once()
	two := registerMock("two", NodeOrchestrator)
	two.On("Detect").Return(nil).Once()

	d := NewDetector("")
	assert.Len(suite.T(), d.candidates, 2)
	assert.Len(suite.T(), d.detected, 0)

	c, n, err := d.GetPreferred()
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "two", n)
	assert.Equal(suite.T(), two, c)

	one.AssertNumberOfCalls(suite.T(), "Detect", 1)
	two.AssertNumberOfCalls(suite.T(), "Detect", 1)
	assert.Nil(suite.T(), d.candidates)
	assert.Nil(suite.T(), d.detected)
}

func TestDetectorTestSuite(t *testing.T) {
	suite.Run(t, new(DetectorTestSuite))
}
