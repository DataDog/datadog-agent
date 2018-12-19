package py

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	python "github.com/DataDog/go-python3"
)

func TestAddExternalTagsBindings(t *testing.T) {
	gstate := newStickyLock()
	defer gstate.unlock()

	module := python.PyImport_ImportModule("external_host_tags")
	require.NotNil(t, module)
	f := module.GetAttrString("test")
	require.NotNil(t, f)
	// this will add 1 entry to the external host metadata cache
	f.Call(python.PyList_New(0), python.PyDict_New())

	ehp := *(externalhost.GetPayload())
	require.Len(t, ehp, 1)
	tuple := ehp[0]
	require.Len(t, tuple, 2)
	assert.Contains(t, "test-py-localhost", tuple[0])
	eTags := externalhost.ExternalTags{"test-source-type": []string{"tag1", "tag2", "tag3"}}
	assert.Equal(t, tuple[1], eTags)
}
