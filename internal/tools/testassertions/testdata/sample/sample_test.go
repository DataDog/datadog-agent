// Package sample is a fixture for the testassertions tool. It is only parsed, never compiled.
package sample

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const expectedHost = "my-host"

type baseSuite struct {
	e2e.BaseSuite[any]
}

type linuxSuite struct {
	baseSuite
}

func TestLinuxSuite(t *testing.T) {
	e2e.Run(t, &linuxSuite{})
}

func (s *baseSuite) SetupSuite() {
	s.Require().NoError(setup())
}

func (s *baseSuite) TestHostname() {
	out, err := s.Env().Agent.Client.Hostname()
	require.NoError(s.T(), err)
	s.Equal(expectedHost, out, "hostname mismatch")

	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("system.uptime")
		if assert.NoError(c, err) {
			for _, m := range metrics {
				checkTags(c, m.Tags)
			}
		}
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxSuite) TestCondition() {
	require.Eventually(s.T(), func() bool {
		return s.Env().Agent.Client.IsReady()
	}, time.Minute, time.Second)
	s.Env().RemoteHost.MustExecute("ls /etc/datadog-agent")
	withRetry(s.T(), func(t *testing.T) {
		if t == nil {
			t.Fatalf("no t")
		}
	})
}

func checkTags(t assert.TestingT, tags []string) {
	assert.Contains(t, tags, "host:"+expectedHost)
}

func withRetry(t *testing.T, fn func(t *testing.T)) {
	fn(t)
}

func setup() error { return nil }
