package py

import (
	"testing"

	"github.com/sbinet/go-python"
)

func getCheckInstance() *PythonCheck {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	module := python.PyImport_ImportModuleNoBlock("testcheck")
	checkClass := module.GetAttrString("TestCheck")
	check := NewPythonCheck("testcheck", checkClass)
	check.Configure([]byte("foo: bar"))
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
	name := "TestCheck"
	if check.String() != name {
		t.Fatalf("Expected %s, found: %v", name, check)
	}

	check.Instance = nil
	if check.String() != "" {
		t.Fatalf("Expected empty string, found: %v", check)
	}
}

func BenchmarkRun(b *testing.B) {
	check := getCheckInstance()
	for n := 0; n < b.N; n++ {
		check.Run()
	}
}
