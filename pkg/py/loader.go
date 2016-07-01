package py

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/checks"
	"github.com/DataDog/datadog-agent/pkg/loader"
	"github.com/sbinet/go-python"
)

// const things
const agentCheckClassName = "AgentCheck"
const agentCheckModuleName = "checks"

// PythonCheckLoader is a specific loader for checks living in Python modules
type PythonCheckLoader struct {
	agentCheckClass *python.PyObject
}

// NewPythonCheckLoader creates an instance of the Python checks loader
func NewPythonCheckLoader() *PythonCheckLoader {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	agentCheckModule := python.PyImport_ImportModuleNoBlock(agentCheckModuleName)
	if agentCheckModule == nil {
		log.Errorf("Unable to import Python module: %s", agentCheckModuleName)
		return nil
	}

	agentCheckClass := agentCheckModule.GetAttrString(agentCheckClassName)
	if agentCheckClass == nil {
		log.Errorf("Unable to import %s class from Python module: %s", agentCheckClassName, agentCheckModuleName)
		return nil
	}

	return &PythonCheckLoader{agentCheckClass}
}

// Load tries to import a Python module with the same name found in config.Name, searches for
// subclasses of the AgentCheck class and returns the corresponding Check
func (cl *PythonCheckLoader) Load(config loader.CheckConfig) ([]checks.Check, error) {
	checks := []checks.Check{}
	moduleName := config.Name

	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// import python module containing the check
	checkModule := python.PyImport_ImportModuleNoBlock(moduleName)
	if checkModule == nil {
		msg := fmt.Sprintf("Unable to import %v", moduleName)
		log.Warningf(msg)
		python.PyErr_Print() // TODO: remove this or redirect to the Go logger
		python.PyErr_Clear()

		return checks, errors.New(msg)
	}

	// Try to find a class inheriting from AgentCheck within the module
	checkClass := findSubclassOf(cl.agentCheckClass, checkModule)
	if checkClass == nil {
		msg := fmt.Sprintf("Unable to find a check class in the module: %v", python.PyString_AS_STRING(checkModule.Str()))
		log.Warningf(msg)
		return checks, errors.New(msg)
	}

	// Get an AgentCheck for each configuration instance and add it to the registry
	instances, found := config.Data["instances"]
	if !found {
		return checks, errors.New("`instances` keyword not found in configuration data")
	}

	instancesList, _ := instances.([]interface{})
	for _, instanceMap := range instancesList {
		// To be retrocompatible with the Python code, still use an `instance` dictionary
		// to contain the (now) unique instance for the check
		conf := make(loader.RawConfigMap)
		conf["instances"] = []interface{}{instanceMap}
		pyConf, err := ToPythonDict(&conf)
		if err != nil {
			log.Errorf("Error parsing check configuration: %v", err)
			continue
		}

		check := NewPythonCheck(checkClass, pyConf)
		if check != nil {
			log.Infof("Loading check: %v", python.PyString_AsString(checkClass.Str()))
			checks = append(checks, check)
		}
	}

	return checks, nil
}
