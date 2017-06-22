package py

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/sbinet/go-python"

	log "github.com/cihub/seelog"
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
	glock := newStickyLock()
	defer glock.unlock()

	agentCheckModule := python.PyImport_ImportModule(agentCheckModuleName)
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
func (cl *PythonCheckLoader) Load(config check.Config) ([]check.Check, error) {
	checks := []check.Check{}
	moduleName := config.Name

	// import python module containing the check
	// Lock the GIL while working with go-python directly
	glock := newStickyLock()
	checkModule := python.PyImport_ImportModule(moduleName)
	if checkModule == nil {
		defer glock.unlock()
		pyErr, err := glock.getPythonError()
		if err != nil {
			return nil, fmt.Errorf("An error occurred while loading the python module and couldn't be formatted: %v", err)
		}
		return nil, errors.New(pyErr)
	}

	// Try to find a class inheriting from AgentCheck within the module
	checkClass, err := findSubclassOf(cl.agentCheckClass, checkModule)
	glock.unlock()
	if err != nil {
		msg := fmt.Sprintf("Unable to find a check class in the module: %v", err)
		return checks, errors.New(msg)
	}

	// Get an AgentCheck for each configuration instance and add it to the registry
	for _, i := range config.Instances {
		check := NewPythonCheck(moduleName, checkClass)
		// The GIL should be unlocked at this point, `check.Configure` uses its own stickyLock and stickyLocks must not be nested
		if err := check.Configure(i, config.InitConfig); err != nil {
			log.Errorf("py.loader: could not configure check '%s': %s", moduleName, err)
			continue
		}
		checks = append(checks, check)
	}

	return checks, nil
}

func (cl *PythonCheckLoader) String() string {
	return "Python Check Loader"
}

func pyLoaderFactory() check.Loader {
	return NewPythonCheckLoader()
}

func init() {
	loaders.RegisterLoader("python", pyLoaderFactory)
}
