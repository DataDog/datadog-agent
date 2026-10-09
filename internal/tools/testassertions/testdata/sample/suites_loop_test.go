package sample

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/stretchr/testify/assert"
)

type amiSuite struct{ e2e.BaseSuite[any] }

type archSuite struct{ e2e.BaseSuite[any] }

func (s *amiSuite) TestAMI() { assert.Equal(s.T(), "ami-123", "ami-123") }

func (s *archSuite) TestArch() { assert.Equal(s.T(), "arm64", "arm64") }

// table-driven: the suite is a field of each test case
func TestSuitesFromTable(t *testing.T) {
	testCases := []struct {
		name  string
		suite e2e.Suite[any]
	}{
		{name: "ami", suite: &amiSuite{}},
		{name: "arch", suite: &archSuite{}},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			e2e.Run(t, tc.suite)
		})
	}
}

// the suite is the element itself
func TestSuitesFromSlice(t *testing.T) {
	suites := []e2e.Suite[any]{&amiSuite{}, &archSuite{}}
	for _, s := range suites {
		e2e.Run(t, s)
	}
}
