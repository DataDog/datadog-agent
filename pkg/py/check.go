package py

import (
	"errors"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/checks"
	"github.com/DataDog/datadog-agent/pkg/loader"
	"github.com/op/go-logging"
	"github.com/sbinet/go-python"
)

var log = logging.MustGetLogger("datadog-agent")

// const things
const agentCheckClassName = "AgentCheck"
const agentCheckModuleName = "checks"

// PythonCheck represents a Python check, implements `Check` interface
type PythonCheck struct {
	Instance   *python.PyObject
	ModuleName string
	Config     loader.CheckConfig
}

// NewPythonCheck conveniently creates a PythonCheck instance
func NewPythonCheck(class *python.PyObject, config loader.CheckConfig) *PythonCheck {
	// pack arguments
	kwargs, _ := ToPythonDict(&config)

	// Lock the GIL and release it at the end
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// invoke constructor
	emptyTuple := python.PyTuple_New(0)
	instance := class.Call(emptyTuple, kwargs)
	if instance == nil {
		python.PyErr_Print()
		return nil
	}

	modName := python.PyString_AsString(instance.GetAttrString("__module__"))

	return &PythonCheck{Instance: instance, ModuleName: modName, Config: config}
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

// CollectChecks return an array of checks to be performed
func CollectChecks(configs []loader.CheckConfig) []checks.Check {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	checks := []checks.Check{}

	agentCheckModule := python.PyImport_ImportModuleNoBlock(agentCheckModuleName)
	if agentCheckModule == nil {
		log.Errorf("Unable to import Python module: %s", agentCheckModuleName)
		return checks
	}

	agentCheckClass := agentCheckModule.GetAttrString(agentCheckClassName)
	if agentCheckClass == nil {
		log.Errorf("Unable to import %s class from Python module: %s", agentCheckClassName, agentCheckModuleName)
		return checks
	}

	for _, config := range configs {
		moduleName := config.Name

		// import python module containing the check
		checkModule := python.PyImport_ImportModuleNoBlock(moduleName)
		if checkModule == nil {
			log.Warningf("Unable to import %v", moduleName)
			python.PyErr_Print() // TODO: remove this or redirect to the Go logger
			python.PyErr_Clear()
			continue
		}

		// Try to find a class inheriting from AgentCheck within the module
		checkClass := findSubclassOf(agentCheckClass, checkModule)
		if checkClass == nil {
			log.Warningf("Unable to find a check class in the module %v", checkModule)
			continue
		}

		// Get an AgentCheck instance and add it to the registry
		check := NewPythonCheck(checkClass, config)
		if check != nil {
			log.Infof("Found check: %v", python.PyString_AsString(checkClass.Str()))
			checks = append(checks, check)
		}
	}

	return checks
}
