package py

import (
	"errors"
	"runtime"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/checks"
	"github.com/DataDog/datadog-agent/pkg/loader"
	"github.com/op/go-logging"
	"github.com/sbinet/go-python"
)

var log = logging.MustGetLogger("datadog-agent")

// PythonCheck represents a Python check, implements `Check` interface
type PythonCheck struct {
	Instance   *python.PyObject
	Class      *python.PyObject
	ModuleName string
	Config     *python.PyObject
}

// NewPythonCheck conveniently creates a PythonCheck instance
func NewPythonCheck(name string, class *python.PyObject) *PythonCheck {
	return &PythonCheck{ModuleName: name, Class: class}
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

// Configure the Python check from YAML data
func (c *PythonCheck) Configure(data checks.ConfigData) {
	// Unmarshal ConfigData to a RawConfigMap
	raw := loader.RawConfigMap{}
	err := yaml.Unmarshal(data, &raw)
	if err != nil {
		// TODO log error
		return
	}

	// Lock the GIL and release it at the end
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// To be retrocompatible with the Python code, still use an `instance` dictionary
	// to contain the (now) unique instance for the check
	conf := make(loader.RawConfigMap)
	conf["instances"] = []interface{}{raw}

	// Convert the RawConfigMap to a Python dictionary
	configDict, err := ToPythonDict(&conf)
	if err != nil {
		log.Errorf("Error parsing check configuration: %v", err)
		return
	}

	// invoke constructor
	emptyTuple := python.PyTuple_New(0)
	instance := c.Class.Call(emptyTuple, configDict)
	if instance == nil {
		python.PyErr_Print()
		// TODO: log Go error
		return
	}

	c.Instance = instance
	c.ModuleName = python.PyString_AsString(instance.GetAttrString("__module__"))
	c.Config = configDict
}
