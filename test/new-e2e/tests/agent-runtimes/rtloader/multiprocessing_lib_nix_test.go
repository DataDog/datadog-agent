package rtloader

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type linuxMultiProcessingLibSuite struct {
	baseMultiProcessingLibSuite
}

func TestLinuxMultiProcessingLibSuite(t *testing.T) {
	t.Parallel()
	suite := &linuxMultiProcessingLibSuite{baseMultiProcessingLibSuite{
		confdPath:   "/etc/datadog-agent/conf.d/multi_file_check.yaml",
		checksdPath: "/etc/datadog-agent/checks.d/multi_file_check.py",
		tempDir:     "/tmp/multi_file_check",
	}}
	e2e.Run(t, suite, suite.getSuiteOptions(os.UbuntuDefault)...)
}
