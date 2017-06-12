package py

import (
	"os"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Setup the test module
func TestMain(m *testing.M) {
	state := Initialize(".", "tests", "../dist")

	// testing this package needs an inited aggregator
	// to work properly
	aggregator.InitAggregator(nil, "")

	ret := m.Run()

	python.PyEval_RestoreThread(state)
	// benchmarks don't like python.Finalize() for some reason, let's just not call it

	os.Exit(ret)
}

func TestInitialize(t *testing.T) {
	key := path.Join(util.AgentCachePrefix, "pythonVersion")
	x, found := util.Cache.Get(key)
	require.True(t, found)
	require.NotEmpty(t, x)
	require.NotEqual(t, "n/a", x)
}

func TestFindSubclassOf(t *testing.T) {
	gstate := newStickyLock()
	defer gstate.unlock()

	fooModule := python.PyImport_ImportModuleNoBlock("foo")
	fooClass := fooModule.GetAttrString("Foo")
	barModule := python.PyImport_ImportModuleNoBlock("bar")
	barClass := barModule.GetAttrString("Bar")

	// invalid input
	sclass, err := findSubclassOf(nil, nil)
	assert.NotNil(t, err)

	// pass something that's not a Type
	sclass, err = findSubclassOf(python.PyTuple_New(0), fooModule)
	assert.NotNil(t, err)
	sclass, err = findSubclassOf(fooClass, python.PyTuple_New(0))
	assert.NotNil(t, err)

	// Foo in foo module, only Foo itself found
	sclass, err = findSubclassOf(fooClass, fooModule)
	assert.NotNil(t, err)

	// Bar in foo module, no class found
	sclass, err = findSubclassOf(barClass, fooModule)
	assert.NotNil(t, err)

	// Foo in bar module, get Bar
	sclass, err = findSubclassOf(fooClass, barModule)
	assert.Nil(t, err)
	assert.Equal(t, 1, sclass.RichCompareBool(barClass, python.Py_EQ))
}

func TestGetModuleName(t *testing.T) {
	name := getModuleName("foo.bar.baz")
	if name != "baz" {
		t.Fatalf("Expected baz, found: %s", name)
	}

	name = getModuleName("baz")
	if name != "baz" {
		t.Fatalf("Expected baz, found: %s", name)
	}

	name = getModuleName("")
	if name != "" {
		t.Fatalf("Expected empty string, found: %s", name)
	}
}
