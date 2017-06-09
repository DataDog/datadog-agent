package metadata

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
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

func TestNewScheduler(t *testing.T) {
	fwd := forwarder.NewDefaultForwarder(nil)
	fwd.Start()
	c := NewScheduler(fwd, "apikey", "hostname")
	assert.Equal(t, "apikey", c.apikey)
	assert.Equal(t, "hostname", c.hostname)
}
