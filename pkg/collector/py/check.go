package py

import (
	"errors"
	"fmt"
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
	gstate := newStickyLock()
	defer gstate.unlock()

	// call run function, it takes no args so we pass an empty tuple
	emptyTuple := python.PyTuple_New(0)
	result := c.Instance.CallMethod("run", emptyTuple)
	if result == nil {
		pyErr, err := gstate.getPythonError()
		if err != nil {
			return fmt.Errorf("An error occurred while running python check and couldn't be formatted: %v", err)
		}
		return errors.New(pyErr)
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

// getInstance invokes the constructor on the Python class stored in
// `c.Class` passing a tuple for args and a dictionary for keyword args.
//
// This function contains deferred calls to go-python: when you change
// this code, please ensure the Python thread unlock is always at the bottom
// of  the defer calls stack.
func (c *PythonCheck) getInstance(args, kwargs *python.PyObject) (*python.PyObject, error) {
	// Lock the GIL and release it at the end
	gstate := newStickyLock()
	defer gstate.unlock()

	if args == nil {
		args = python.PyTuple_New(0)
	}

	// invoke class constructor
	instance := c.Class.Call(args, kwargs)
	if instance != nil {
		return instance, nil
	}

	// there was an error, retrieve it
	pyErr, err := gstate.getPythonError()
	if err != nil {
		return nil, fmt.Errorf("An error occurred while invoking the python check constructor, and couldn't be formatted: %v", err)
	}
	return nil, errors.New(pyErr)
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

	// To be retrocompatible with the Python code, still use an `instance` dictionary
	// to contain the (now) unique instance for the check
	conf := make(check.ConfigRawMap)
	conf["name"] = c.ModuleName
	conf["init_config"] = rawInitConfig
	conf["instances"] = []interface{}{rawInstances}

	// Convert the RawConfigMap to a Python dictionary
	kwargs, err := ToPython(&conf)
	if err != nil {
		log.Errorf("Error parsing python check configuration: %v", err)
		return err
	}

	// try getting an instance with the new style api, without passing agentConfig
	instance, err := c.getInstance(nil, kwargs)
	if err != nil {
		log.Warnf("could not get a check instance with the new api: %s", err)
		log.Warn("trying to instantiate the check with the old api, passing initConfig to the constructor")

		// try again, assuming the check is good but has still the old api
		// we pass initConfig but emit a deprecation notice
		allSettings := config.Datadog.AllSettings()
		agentConfig, err := ToPython(allSettings)
		if err != nil {
			return fmt.Errorf("could not convert agent configuration to python: %s", err)
		}

		// Add new 'agentConfig' key to the dict...
		gstate := newStickyLock()
		key := python.PyString_FromString("agentConfig")
		python.PyDict_SetItem(kwargs, key, agentConfig)
		gstate.unlock()

		// ...and retry to get an instance
		instance, err = c.getInstance(nil, kwargs)
		if err != nil {
			return fmt.Errorf("could not invoke python check constructor: %s", err)
		}

		log.Warnf("passing `agentConfig` to the constructor is deprecated, please use the `get_config` function from the 'datadog_agent' package (%s).", c.ModuleName)
	}

	c.Instance = instance
	c.Config = kwargs
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
