// NOTICE: See TestMain function in `utils_test.go` for Python initialization
package py

import (
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/mitchellh/reflectwalk"
	"github.com/sbinet/go-python"

	"gopkg.in/yaml.v2"
)

// Check that the result of the func is the same as the dict contained in the test directory,
// computed with PyYAML
func TestToPythonDict(t *testing.T) {
	yamlFile, err := ioutil.ReadFile("tests/complex.yaml")
	if err != nil {
		t.Fatalf("Expected empty error message, found: %s", err)
	}

	c := make(check.ConfigRawMap)

	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		t.Fatalf("Expected empty error message, found: %s", err)
	}

	res, err := ToPythonDict(&c)
	if err != nil {
		t.Fatalf("Expected empty error message, found: %s", err)
	}

	// Lock the GIL and release it at the end
	_gstate := python.PyGILState_Ensure()

	if !python.PyDict_Check(res) {
		t.Fatalf("Result is not a Python dict")
	}

	m := python.PyImport_ImportModuleNoBlock("tests.complex")
	if m == nil {
		t.Fatalf("Unable to import module complex.py")
	}

	d := m.GetAttrString("d")
	if d == nil {
		t.Fatalf("Unable to import test dictionary from module complex.py")
	}

	same := res.RichCompareBool(d, python.Py_EQ)
	if same < 1 {
		t.Fatalf("Result and template dict must be the same:\n Result: %s\n Template: %s",
			python.PyString_AsString(res.Str()),
			python.PyString_AsString(d.Str()))
	}

	python.PyGILState_Release(_gstate)
}

func TestWalkerPush(t *testing.T) {
	w := new(walker)
	d := python.PyDict_New()

	// initial state
	w.push(d)
	if len(w.containersStack) != 1 {
		t.Fatalf("Stack should have size 1, found: %d", len(w.containersStack))
	}
	if w.currentContainer != w.containersStack[0] {
		t.Fatalf("Initial state not consistent, found %v", w)
	}
	if w.currentContainer != d {
		t.Fatalf("Initial state not consistent, found %v", w)
	}

	// push again
	d2 := python.PyDict_New()
	w.push(d2)
	if len(w.containersStack) != 2 {
		t.Fatalf("Stack should have size 2, found: %d", len(w.containersStack))
	}
	if w.currentContainer != d2 {
		t.Fatalf("Initial state not consistent, found %v", w)
	}

	// and again
	l := python.PyList_New(0)
	w.push(l)
	if len(w.containersStack) != 3 {
		t.Fatalf("Stack should have size 3, found: %d", len(w.containersStack))
	}
	if w.currentContainer != l {
		t.Fatalf("Initial state not consistent, found %v", w)
	}
}

func TestWalkerPop(t *testing.T) {
	w := new(walker)
	d := python.PyDict_New()
	w.push(d)
	w.push(python.PyDict_New())
	w.pop()
	if len(w.containersStack) != 1 {
		t.Fatalf("Stack should have size 1, found: %d", len(w.containersStack))
	}
	if w.currentContainer != d {
		t.Fatalf("Initial state not consistent, found %v", w)
	}

	// empty stack
	w.pop()

	// try to pop from an empty stack
	w.pop()
	if w.currentContainer != d {
		t.Fatalf("Initial state not consistent, found %v", w)
	}
}

func TestWalkerEnter(t *testing.T) {
	w := new(walker)
	w.Enter(reflectwalk.Map)
	if len(w.containersStack) != 1 {
		t.Fatalf("Stack should have size 1, found: %d", len(w.containersStack))
	}
	if !python.PyDict_Check(w.currentContainer) {
		t.Fatalf("Current container is not a dict")
	}

	w.Enter(reflectwalk.Slice)
	if len(w.containersStack) != 2 {
		t.Fatalf("Stack should have size 2, found: %d", len(w.containersStack))
	}
	if python.PyDict_Check(w.currentContainer) {
		t.Fatalf("Current container is a dict, shouldnt be")
	}
}

func TestWalkerExit(t *testing.T) {
	w := new(walker)
	w.lastKey = "foo"
	w.Exit(reflectwalk.Map)
	if len(w.containersStack) != 0 {
		t.Fatalf("Stack should have size 0, found: %d", len(w.containersStack))
	}
	if w.lastKey != "" {
		t.Fatalf("Last key should be empty, found: %s", w.lastKey)
	}

	w = new(walker)
	w.lastKey = "bar"
	w.Exit(reflectwalk.Map)
	if len(w.containersStack) != 0 {
		t.Fatalf("Stack should have size 0, found: %d", len(w.containersStack))
	}
	if w.lastKey != "" {
		t.Fatalf("Last key should be empty, found: %s", w.lastKey)
	}

	// fill the stack to test cleanup procedure
	w.push(python.PyDict_New())
	w.Exit(reflectwalk.WalkLoc)
	if w.containersStack != nil {
		t.Fatalf("Stack should be nil, found: %v", w.containersStack)
	}
	if w.currentContainer != nil {
		t.Fatalf("Current container should be nil, found: %v", w.currentContainer)
	}
}

func TestIfToPy(t *testing.T) {
	var i interface{}

	i = 42
	val := ifToPy(reflect.ValueOf(i))
	if !python.PyInt_Check(val) {
		t.Fatalf("Return value is not int")
	}

	i = "Snafu"
	val = ifToPy(reflect.ValueOf(i))
	if !python.PyString_Check(val) {
		t.Fatalf("Return value is not a string")
	}

	i = false
	val = ifToPy(reflect.ValueOf(i))
	if !python.PyBool_Check(val) {
		t.Fatalf("Return value is not bool")
	}

	var nilInt *int
	val = ifToPy(reflect.ValueOf(nilInt))
	if val != python.Py_None {
		t.Fatalf("Return value is not None")
	}
}

func TestMapElem(t *testing.T) {
	k := "foo"
	v := "bar"
	m := map[string]string{}
	w := new(walker)
	w.currentContainer = python.PyDict_New()
	w.MapElem(reflect.ValueOf(m), reflect.ValueOf(k), reflect.ValueOf(v))
	if w.lastKey != k {
		t.Fatalf("Expected key value foo, found %s", w.lastKey)
	}

	pkey := python.PyString_FromString(w.lastKey)
	ok, _ := python.PyDict_Contains(w.currentContainer, pkey)
	if !ok {
		t.Fatalf("Key not found in dictionary")
	}
}

func TestSliceElem(t *testing.T) {
	v := "bar"
	w := new(walker)
	w.currentContainer = python.PyList_New(0)
	w.SliceElem(0, reflect.ValueOf(v))
	l := python.PyList_Size(w.currentContainer)
	if l != 1 {
		t.Fatalf("Expected list lenght 1, found %d", l)
	}
}
