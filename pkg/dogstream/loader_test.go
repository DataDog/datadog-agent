package dogstream

import (
	"os"
	"testing"

	py "github.com/DataDog/datadog-agent/pkg/collector/check/py"
	python "github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
)

// Setup the test module
func TestMain(m *testing.M) {
	state := py.Initialize("tests")

	ret := m.Run()

	python.PyEval_RestoreThread(state)
	python.Finalize()

	os.Exit(ret)
}

func TestLoad(t *testing.T) {
	p, err := Load("foo")
	assert.Nil(t, err)

	assert.Nil(t, p.Parse("foo", "bar"))
}
