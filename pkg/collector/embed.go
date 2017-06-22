// +build cpython

package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/py"
	python "github.com/sbinet/go-python"
)

var pyState *python.PyThreadState

func pySetup(paths ...string) {
	pyState = py.Initialize(paths...)
}

func pyTeardown() {
	python.PyEval_RestoreThread(pyState)
	pyState = nil
}
