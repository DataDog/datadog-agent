package metadata

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check/py"
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

func TestNewCollector(t *testing.T) {
	fwd := forwarder.NewForwarder(nil)
	fwd.Start()
	c := NewCollector(fwd, "apikey", "hostname")
	assert.NotNil(t, c.sendHostT)
	assert.NotNil(t, c.sendExtHostT)
	assert.NotNil(t, c.sendAgentCheckT)
	assert.NotNil(t, c.sendProcessesT)
	assert.Equal(t, "apikey", c.apikey)
	assert.Equal(t, "hostname", c.hostname)
}
