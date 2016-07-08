package py

import (
	"strings"

	"github.com/sbinet/go-python"
)

// Initialize wraps all the operations needed to start the Python interpreter and
// configure the environment
func Initialize(paths ...string) *python.PyThreadState {
	// Start the interpreter
	err := python.Initialize()
	if err != nil {
		panic(err.Error())
	}

	// Set the PYTHONPATH if needed
	if len(paths) > 0 {
		path := python.PySys_GetObject("path")
		for _, p := range paths {
			python.PyList_Append(path, python.PyString_FromString(p))
		}
	}

	// `python.Initialize` acquires the GIL but from this point
	// we don't need it anymore, let's release it
	state := python.PyEval_SaveThread()

	// inject the aggregator into global namespace
	// (it will handle the GIL by itself)
	initApi()

	// return the state so the caller can resume
	return state
}

// Search in module for a class deriving from baseClass and return the first match if any.
func findSubclassOf(base, module *python.PyObject) *python.PyObject {
	// baseClass is not a Class type
	if base == nil || !python.PyType_Check(base) {
		return nil
	}

	// module is not a Module object
	if module == nil || !python.PyModule_Check(module) {
		return nil
	}

	dir := module.PyObject_Dir()
	var class *python.PyObject
	for i := 0; i < python.PyList_GET_SIZE(dir); i++ {
		symbolName := python.PyString_AsString(python.PyList_GET_ITEM(dir, i))
		class = module.GetAttrString(symbolName)

		if !python.PyType_Check(class) {
			continue
		}

		// IsSubclass returns success if class is the same, we need to go deeper
		if class.IsSubclass(base) == 1 && class.RichCompareBool(base, python.Py_EQ) != 1 {
			return class
		}
	}
	return nil
}

// Get the rightmost component of a module path like foo.bar.baz
func getModuleName(modulePath string) string {
	toks := strings.Split(modulePath, ".")
	// no need to check toks length, worst case it contains only an empty string
	return toks[len(toks)-1]
}
