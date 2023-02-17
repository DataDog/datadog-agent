package submitter

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/process/runner/mocks"
)

func newMock(deps dependencies, t testing.TB) Component {
	return mocks.NewSubmitter(t)
}
