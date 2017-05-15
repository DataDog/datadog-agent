package v5

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/py"
	python "github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
)

// Setup the test module
func TestMain(m *testing.M) {
	state := py.Initialize()

	ret := m.Run()

	python.PyEval_RestoreThread(state)
	python.Finalize()

	os.Exit(ret)
}

func TestFoo(t *testing.T) {
	pl := GetPayload("testhostname")
	assert.NotNil(t, pl)
}
