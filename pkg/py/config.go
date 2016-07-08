package py

import (
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/mitchellh/reflectwalk"
	"github.com/sbinet/go-python"
)

// we use this struct to walk through YAML results and convert them to Python stuff
type walker struct {
	result           *python.PyObject
	lastKey          string
	containersStack  []*python.PyObject
	currentContainer *python.PyObject
}

// ToPythonDict dumps a RawConfigMap into a Python dictionary
func ToPythonDict(m *check.ConfigRawMap) (*python.PyObject, error) {
	w := new(walker)
	err := reflectwalk.Walk(m, w)

	return w.result, err
}

// push the old container to the stack and start using the new one
func (w *walker) push(newc *python.PyObject) {
	// special case: init
	if w.result == nil {
		w.containersStack = append(w.containersStack, newc)
		w.currentContainer = newc
		w.result = newc
		return
	}

	// Lock the GIL and release it at the end
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// add new container to the current one before pushing it to the stack
	// we can safely assume it's either a `list` or a `dict`
	if python.PyDict_Check(w.currentContainer) {
		k := python.PyString_FromString(w.lastKey)
		python.PyDict_SetItem(w.currentContainer, k, newc)
	} else {
		python.PyList_Append(w.currentContainer, newc)
	}

	// push it like there's no tomorrow
	w.containersStack = append(w.containersStack, w.currentContainer)
	w.currentContainer = newc
}

// pop an old container and start adding new stuff to it
// do nothing if the stack is empty
func (w *walker) pop() {
	l := len(w.containersStack)
	if l > 0 {
		w.currentContainer = w.containersStack[l-1]
		w.containersStack = w.containersStack[:l-1]
	}
}

// the walker is about to enter a new type, we only need to take action for
// Maps and Slices.
func (w *walker) Enter(l reflectwalk.Location) error {

	// Lock the GIL and release it at the end
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	switch l {
	case reflectwalk.Map:
		// push a new map on the stack
		w.push(python.PyDict_New())
	case reflectwalk.Slice:
		// push a new list on the stack
		w.push(python.PyList_New(0))
	}

	return nil
}

// the walker has done with the previous type, pop an old container
// from the stack and reset the last seen dict key
func (w *walker) Exit(l reflectwalk.Location) error {
	switch l {
	case reflectwalk.Map:
		fallthrough
	case reflectwalk.Slice:
		// Pop previous container
		w.lastKey = ""
		w.pop()
	case reflectwalk.WalkLoc:
		// Cleanup
		w.containersStack = nil
		w.currentContainer = nil
	}

	return nil
}

// only to implement Walker interface
func (w *walker) Map(m reflect.Value) error {
	return nil
}

// only to implement Walker interface
func (w *walker) Slice(s reflect.Value) error {
	return nil
}

// ugly but YAML returns only interfaces, need to introspect manually
func ifToPy(v reflect.Value) *python.PyObject {

	// Lock the GIL and release it at the end
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	var pyval *python.PyObject
	vi := v.Interface()

	if s, ok := vi.(string); ok {
		pyval = python.PyString_FromString(s)
	} else if s, ok := vi.(int); ok {
		pyval = python.PyInt_FromLong(int(s))
	} else if s, ok := vi.(bool); ok {
		if s {
			pyval = python.PyBool_FromLong(1)
		} else {
			pyval = python.PyBool_FromLong(0)
		}
	} else if v.IsNil() {
		pyval = python.Py_None
	}

	return pyval
}

// go through map elements and convert to Python dict elements
func (w *walker) MapElem(m, k, v reflect.Value) error {

	// Lock the GIL and release it at the end
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	w.lastKey = k.Interface().(string)
	dictKey := python.PyString_FromString(w.lastKey)

	// set the converted value in the Python dict
	pyval := ifToPy(v)
	if pyval != nil {
		python.PyDict_SetItem(w.currentContainer, dictKey, pyval)
	}

	return nil
}

// go through slice items and convert to Python list items
func (w *walker) SliceElem(i int, v reflect.Value) error {

	// Lock the GIL and release it at the end
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	pyval := ifToPy(v)
	if pyval != nil {
		python.PyList_Append(w.currentContainer, pyval)
	}

	return nil
}
