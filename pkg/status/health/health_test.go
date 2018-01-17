// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package health

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type HealthTestSuite struct {
	suite.Suite
}

// put configuration back in a known state before each test
func (s *HealthTestSuite) SetupTest() {
	reset()
}

func (s *HealthTestSuite) TestRegisterAndUnhealthy() {
	token := Register("test1")

	c, found := catalog.components[token]
	require.True(s.T(), found)

	assert.Equal(s.T(), "test1", c.name)
	assert.EqualValues(s.T(), 30, c.timeout.Seconds())
	assert.True(s.T(), time.Now().After(c.latestPing))

	status := Status()
	assert.Len(s.T(), status.Healthy, 0)
	assert.Len(s.T(), status.UnHealthy, 1)
	assert.Contains(s.T(), status.UnHealthy, "test1")
}

func (s *HealthTestSuite) TestRegisterCustomTimeout() {
	token := RegisterWithCustomTimeout("test1", 90*time.Second)

	c, found := catalog.components[token]
	require.True(s.T(), found)

	assert.Equal(s.T(), "test1", c.name)
	assert.EqualValues(s.T(), 90, c.timeout.Seconds())
}

func (s *HealthTestSuite) TestDeregister() {
	token1 := Register("test1")
	token2 := Register("test2")

	assert.Len(s.T(), catalog.components, 2)

	err := Deregister(token1)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), catalog.components, 1)
	assert.Contains(s.T(), catalog.components, token2)
}

func (s *HealthTestSuite) TestDeregisterBadToken() {
	token1 := Register("test1")

	assert.Len(s.T(), catalog.components, 1)

	err := Deregister("invalid")
	assert.NotNil(s.T(), err)
	assert.Len(s.T(), catalog.components, 1)
	assert.Contains(s.T(), catalog.components, token1)
}

func (s *HealthTestSuite) TestPing() {
	token := Register("test")
	c := catalog.components[token]
	assert.True(s.T(), time.Now().After(c.latestPing.Add(c.timeout)))

	err := Ping(token)
	assert.NoError(s.T(), err)
	assert.False(s.T(), time.Now().After(c.latestPing.Add(c.timeout)))
}

func (s *HealthTestSuite) TestPingNotRegistered() {
	err := Ping("invalid")
	assert.Error(s.T(), err)
}

func (s *HealthTestSuite) TestUnhealthyAndBack() {
	token := Register("test")
	status := Status()
	assert.NotContains(s.T(), status.Healthy, "test")
	assert.Contains(s.T(), status.UnHealthy, "test")

	// Become healthy
	Ping(token)
	status = Status()
	assert.NotContains(s.T(), status.UnHealthy, "test")
	assert.Contains(s.T(), status.Healthy, "test")

	// Become unhealthy
	registerPing(token, time.Now().Add(-10*time.Minute))
	status = Status()
	assert.NotContains(s.T(), status.Healthy, "test")
	assert.Contains(s.T(), status.UnHealthy, "test")

	// Become healthy again
	Ping(token)
	status = Status()
	assert.NotContains(s.T(), status.UnHealthy, "test")
	assert.Contains(s.T(), status.Healthy, "test")
}

func TestHealthSuite(t *testing.T) {
	suite.Run(t, &HealthTestSuite{})
}
