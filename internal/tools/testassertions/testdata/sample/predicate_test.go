package sample

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type orchSuite struct {
	e2e.BaseSuite[any]
}

// explicit type argument on e2e.Run
func TestGenericRun(t *testing.T) {
	e2e.Run[any](t, &orchSuite{})
}

type expectResource struct {
	test    func(kind string) bool
	message string
}

// Assert polls until a payload satisfies the test predicate.
func (e expectResource) Assert(t *testing.T, kinds []string) {
	for _, k := range kinds {
		if e.test(k) {
			return
		}
	}
	t.Error("failed to " + e.message)
}

func (s *orchSuite) TestPod() {
	expectResource{
		test:    func(kind string) bool { return kind == "Pod" },
		message: "find a pod",
	}.Assert(s.T(), nil)
}
