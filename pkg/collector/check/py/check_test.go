// NOTICE: See TestMain function in `utils_test.go` for Python initialization
// FIXME migrate to testify ASAP

package py

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
)

func getCheckInstance() *PythonCheck {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	aggregator.GetAggregator()

	module := python.PyImport_ImportModuleNoBlock("testcheck")
	checkClass := module.GetAttrString("TestCheck")
	check := NewPythonCheck("testcheck", checkClass)
	check.Configure([]byte("foo: bar"))
	check.InitSender()
	return check
}

func TestNewPythonCheck(t *testing.T) {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	tuple := python.PyTuple_New(0)
	res := NewPythonCheck("FooBar", tuple)

	if res.Class != tuple {
		t.Fatalf("Expected %v, found: %v", tuple, res.Class)
	}

	if res.ModuleName != "FooBar" {
		t.Fatalf("Expected FooBar, found: %v", res.ModuleName)
	}
}

// TODO check arguments as soon as the feature is complete
func _TestNewPythonCheck(t *testing.T) {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// module := python.PyImport_ImportModuleNoBlock("testcheck")
	// checkClass := module.GetAttrString("TestCheck")
	// check := NewPythonCheck(checkClass, python.PyTuple_New(0))
	//
	// if check.Instance.IsInstance(checkClass) != 1 {
	// 	t.Fatalf("Expected instance of class TestCheck, found: %s",
	// 		python.PyString_AsString(check.Instance.GetAttrString("__class__")))
	// }
	//
	// // this should fail b/c FooCheck constructors takes parameters
	// fooClass := module.GetAttrString("FooCheck")
	// check = NewPythonCheck(fooClass, python.PyTuple_New(0))
	//
	// if check != nil {
	// 	t.Fatalf("nil expected, found: %v", check)
	// }
}

func TestRun(t *testing.T) {
	check := getCheckInstance()
	if err := check.Run(); err != nil {
		t.Fatalf("Expected error nil, found: %s", err)
	}
}

func TestStr(t *testing.T) {
	check := getCheckInstance()
	name := "testcheck"
	if check.String() != name {
		t.Fatalf("Expected %s, found: %v", name, check)
	}
}

func TestInterval(t *testing.T) {
	c := getCheckInstance()
	assert.Equal(t, check.DefaultCheckInterval, c.Interval())
	c.Configure([]byte("min_collection_interval: 1"))
	assert.Equal(t, time.Duration(1), c.Interval())
}

func BenchmarkRun(b *testing.B) {
	check := getCheckInstance()
	for n := 0; n < b.N; n++ {
		check.Run()
	}
}
