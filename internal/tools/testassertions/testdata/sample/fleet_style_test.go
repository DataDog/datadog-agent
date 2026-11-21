package sample

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/stretchr/testify/assert"
)

type configSuite struct {
	e2e.BaseSuite[any]
}

// newConfigSuite is declared to return an interface: the concrete suite type
// is only visible in the return statement.
func newConfigSuite() e2e.Suite[any] {
	return &configSuite{}
}

// runOnPlatforms mimics fleet's suite.Run: it receives a suite constructor.
func runOnPlatforms(t *testing.T, f func() e2e.Suite[any], platforms []string) {
	for _, platform := range platforms {
		s := f()
		t.Run(platform, func(t *testing.T) {
			e2e.Run(t, s)
		})
	}
}

func TestFleetStyle(t *testing.T) {
	runOnPlatforms(t, newConfigSuite, []string{"ubuntu", "debian"})
}

type section struct {
	name          string
	shouldContain []string
}

const configKey = "logs_enabled"

func (s *configSuite) TestSections() {
	sections := []section{{name: "Collector", shouldContain: []string{"Running Checks", configKey}}}
	for _, sec := range sections {
		checkSection(s.T(), sec)
	}
	checkSection(s.T(), section{name: "Forwarder", shouldContain: []string{"Transactions"}})
}

func checkSection(t *testing.T, sec section) {
	assert.NotEmpty(t, sec.name)
	for _, want := range sec.shouldContain {
		assert.Contains(t, "status output", want)
	}
}

// a one-assertion helper called twice: the second call is collapsed too
func (s *configSuite) TestSmallHelperTwice() {
	requireService(s.T(), "datadog-agent")
	requireService(s.T(), "datadog-agent-sysprobe")
}

func requireService(t *testing.T, name string) {
	assert.NotEmpty(t, name)
}
