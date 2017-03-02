package py

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/sbinet/go-python"

	log "github.com/cihub/seelog"
)

// #include <Python.h>
import "C"

// PythonCheck represents a Python check, implements `Check` interface
type PythonCheck struct {
	id         check.ID
	Instance   *python.PyObject
	Class      *python.PyObject
	ModuleName string
	Config     *python.PyObject
	interval   time.Duration
}

// NewPythonCheck conveniently creates a PythonCheck instance
func NewPythonCheck(name string, class *python.PyObject) *PythonCheck {
	return &PythonCheck{
		ModuleName: name,
		Class:      class,
		interval:   check.DefaultCheckInterval,
	}
}

// Run a Python check
func (c *PythonCheck) Run() error {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	runtime.LockOSThread()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// call run function, it takes no args so we pass an empty tuple
	emptyTuple := python.PyTuple_New(0)
	result := c.Instance.CallMethod("run", emptyTuple)
	if result == nil {
		python.PyErr_Print()
		return errors.New("Unable to run Python check")
	}

	s, err := aggregator.GetDefaultSender()
	if err != nil {
		return fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}

	s.Commit()

	var resultStr = python.PyString_AsString(result)
	if resultStr == "" {
		return nil
	}

	return errors.New(resultStr)
}

// Stop does nothing
func (c *PythonCheck) Stop() {}

// String representation (for debug and logging)
func (c *PythonCheck) String() string {
	return c.ModuleName
}

func (c *PythonCheck) instantiateCheck(constructorParameters *python.PyObject, fixInitError bool) (*python.PyObject, error) {
	// invoke constructor
	emptyTuple := python.PyTuple_New(0)
	instance := c.Class.Call(emptyTuple, constructorParameters)
	if instance != nil {
		return instance, nil
	}

	// If the constructor is invalid we do not get a traceback but
	// an error in pvalue.
	_, pvalue, ptraceback := python.PyErr_Fetch()

	// The internal C pointer may be nill, the only way to check is
	// it to ask for the type, since the internal "ptr" is not
	// exposed.
	if pvalue.Type() != nil {
		pvalueError := python.PyString_AsString(pvalue)

		// 'agentConfig' has been deprecated since agent 6.0.
		// Until it's removed we try do detect error from
		// __init__ missing one parameter and try to load the
		// check again with an empty 'agentConfig' (since most
		// user custom check expect the argument but don't use
		// it).
		if fixInitError && strings.HasPrefix(pvalueError, "__init__() takes ") {
			conf := config.Datadog.AllSettings()
			agentConfig, err := ToPythonDict(conf)
			if err != nil {
				return nil, fmt.Errorf("could not convert agent configuration to python: %s", err)
			}

			// Add new 'agentConfig' key to the dict and retry instantiateCheck
			key := python.PyString_FromString("agentConfig")
			python.PyDict_SetItem(constructorParameters, key, agentConfig)
			instance, err := c.instantiateCheck(constructorParameters, false)
			if instance != nil {
				log.Warnf("'agentConfig' parameter in the '__init__' method is deprecated, please use the agent_config python package (%s).", c.ModuleName)
			}
			return instance, err
		}

		return nil, fmt.Errorf(pvalueError)
	}
	if ptraceback.Type() != nil {
		return nil, fmt.Errorf(python.PyString_AsString(ptraceback))
	}

	return nil, fmt.Errorf("unknown error from python")
}

// Configure the Python check from YAML data
func (c *PythonCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// Generate check ID
	c.id = check.Identify(c, data, initConfig)

	// Unmarshal instances config to a RawConfigMap
	rawInstances := check.ConfigRawMap{}
	err := yaml.Unmarshal(data, &rawInstances)
	if err != nil {
		log.Errorf("error in yaml %s", err)
		return err
	}

	// Unmarshal initConfig to a RawConfigMap
	rawInitConfig := check.ConfigRawMap{}
	err = yaml.Unmarshal(initConfig, &rawInitConfig)
	if err != nil {
		log.Errorf("error in yaml %s", err)
		return err
	}

	// See if a collection interval was specified
	x, ok := rawInstances["min_collection_interval"]
	if ok {
		// we should receive an int from the unmarshaller
		if intl, ok := x.(int); ok {
			// all good, convert to the right type, assuming YAML contains seconds
			c.interval = time.Duration(intl) * time.Second
		}
	}

	// Lock the GIL and release it at the end
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	// To be retrocompatible with the Python code, still use an `instance` dictionary
	// to contain the (now) unique instance for the check
	conf := make(check.ConfigRawMap)
	conf["instances"] = []interface{}{rawInstances}
	conf["init_config"] = rawInitConfig
	conf["name"] = c.ModuleName

	// Convert the RawConfigMap to a Python dictionary
	configDict, err := ToPythonDict(&conf)
	if err != nil {
		log.Errorf("Error parsing python check configuration: %v", err)
		return err
	}

	instance, err := c.instantiateCheck(configDict, true)
	if instance == nil {
		return fmt.Errorf("could not invoke python check constructor: %s", err)
	}

	c.Instance = instance
	c.ModuleName = python.PyString_AsString(instance.GetAttrString("__module__"))
	c.Config = configDict
	return nil
}

// InitSender does nothing here because all python checks use the default sender
func (c *PythonCheck) InitSender() {
}

// Interval returns the scheduling time for the check
func (c *PythonCheck) Interval() time.Duration {
	return c.interval
}

// ID returns the ID of the check
func (c *PythonCheck) ID() check.ID {
	return c.id
}
