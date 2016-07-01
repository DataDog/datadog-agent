package py

import (
	"errors"
	"runtime"

	"github.com/op/go-logging"
	"github.com/sbinet/go-python"
)

var log = logging.MustGetLogger("datadog-agent")

// PythonCheck represents a Python check, implements `Check` interface
type PythonCheck struct {
	Instance   *python.PyObject
	ModuleName string
	Config     *python.PyObject
}

// NewPythonCheck conveniently creates a PythonCheck instance
func NewPythonCheck(class *python.PyObject, configDict *python.PyObject) *PythonCheck {
	// Lock the GIL and release it at the end
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// invoke constructor
	emptyTuple := python.PyTuple_New(0)
	instance := class.Call(emptyTuple, configDict)
	if instance == nil {
		python.PyErr_Print()
		return nil
	}

	modName := python.PyString_AsString(instance.GetAttrString("__module__"))

	return &PythonCheck{Instance: instance, ModuleName: modName, Config: configDict}
}

// Run a Python check
func (c *PythonCheck) Run() error {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	runtime.LockOSThread()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// call run function
	runFunc := c.Instance.GetAttrString("run")
	emptyTuple := python.PyTuple_New(0)
	result := runFunc.CallObject(emptyTuple)
	var resultStr string
	if result == nil {
		python.PyErr_Print()
		return errors.New("Unable to run Python check.")
	}

	resultStr = python.PyString_AsString(result)
	if resultStr == "" {
		return nil
	}

	return errors.New(resultStr)
}

// String representation (for debug and logging)
func (c *PythonCheck) String() string {
	if c.Instance != nil {
		return python.PyString_AsString(c.Instance.GetAttrString("__class__").GetAttrString("__name__"))
	}
	return ""
}
