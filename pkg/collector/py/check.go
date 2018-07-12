// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

package py

import (
	"errors"
	"fmt"
	"runtime"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/sbinet/go-python"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// #include <Python.h>
import "C"

// PythonCheck represents a Python check, implements `Check` interface
type PythonCheck struct {
	id           check.ID
	version      string
	instance     *python.PyObject
	class        *python.PyObject
	ModuleName   string
	config       *python.PyObject
	interval     time.Duration
	lastWarnings []error
}

// NewPythonCheck conveniently creates a PythonCheck instance
func NewPythonCheck(name string, class *python.PyObject) *PythonCheck {
	glock := newStickyLock()
	class.IncRef() // own the ref
	glock.unlock()
	pyCheck := &PythonCheck{
		ModuleName:   name,
		class:        class,
		interval:     check.DefaultCheckInterval,
		lastWarnings: []error{},
	}
	runtime.SetFinalizer(pyCheck, pythonCheckFinalizer)
	return pyCheck
}

// Run a Python check
func (c *PythonCheck) Run() error {
	// Lock the GIL and release it at the end of the run
	gstate := newStickyLock()
	defer gstate.unlock()

	// call run function, it takes no args so we pass an empty tuple
	log.Debugf("Running python check %s %s", c.ModuleName, c.id)
	emptyTuple := python.PyTuple_New(0)
	defer emptyTuple.DecRef()
	result := c.instance.CallMethod("run", emptyTuple)
	log.Debugf("Run returned for %s %s", c.ModuleName, c.id)
	if result == nil {
		pyErr, err := gstate.getPythonError()
		if err != nil {
			return fmt.Errorf("An error occurred while running python check and couldn't be formatted: %v", err)
		}
		return errors.New(pyErr)
	}
	defer result.DecRef()

	s, err := aggregator.GetSender(c.ID())
	if err != nil {
		return fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}

	s.Commit()

	// grab the warnings and add them to the struct
	c.lastWarnings = c.getPythonWarnings(gstate)

	var resultStr = python.PyString_AsString(result)
	if resultStr == "" {
		return nil
	}

	return errors.New(resultStr)
}

// RunSimple runs a Python check without sending data to the aggregator
func (c *PythonCheck) RunSimple() error {
	gstate := newStickyLock()
	defer gstate.unlock()

	log.Debugf("Running python check %s %s", c.ModuleName, c.id)
	emptyTuple := python.PyTuple_New(0)
	defer emptyTuple.DecRef()

	result := c.instance.CallMethod("run", emptyTuple)
	log.Debugf("Run returned for %s %s", c.ModuleName, c.id)
	if result == nil {
		pyErr, err := gstate.getPythonError()
		if err != nil {
			return fmt.Errorf("An error occurred while running python check and couldn't be formatted: %v", err)
		}
		return errors.New(pyErr)
	}
	defer result.DecRef()

	c.lastWarnings = c.getPythonWarnings(gstate)
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

// Version returns the version of the check if load from a python wheel
func (c *PythonCheck) Version() string {
	return c.version
}

// GetWarnings grabs the last warnings from the struct
func (c *PythonCheck) GetWarnings() []error {
	warnings := c.lastWarnings
	c.lastWarnings = []error{}
	return warnings
}

// getPythonWarnings grabs the last warnings from the python check
func (c *PythonCheck) getPythonWarnings(gstate *stickyLock) []error {
	/**
	This function must be run before the GIL is unlocked, otherwise it will return nothing.
	**/
	warnings := []error{}
	emptyTuple := python.PyTuple_New(0)
	defer emptyTuple.DecRef()
	ws := c.instance.CallMethod("get_warnings", emptyTuple)
	if ws == nil {
		pyErr, err := gstate.getPythonError()
		if err != nil {
			log.Errorf("An error occurred while grabbing python check and couldn't be formatted: %v", err)
		}
		log.Infof("Python error: %v", pyErr)
		return warnings
	}
	defer ws.DecRef()
	numWarnings := python.PyList_Size(ws)
	idx := 0
	for idx < numWarnings {
		w := python.PyList_GetItem(ws, idx) // borrowed ref
		warnings = append(warnings, fmt.Errorf("%v", python.PyString_AsString(w)))
		idx++
	}
	return warnings
}

// getInstance invokes the constructor on the Python class stored in
// `c.class` passing a tuple for args and a dictionary for keyword args.
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
		defer args.DecRef()
	}

	// invoke class constructor
	instance := c.class.Call(args, kwargs)
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
func (c *PythonCheck) Configure(data integration.Data, initConfig integration.Data) error {
	// Generate check ID
	c.id = check.Identify(c, data, initConfig)

	// Unmarshal instances config to a RawConfigMap
	rawInstances := integration.RawMap{}
	err := yaml.Unmarshal(data, &rawInstances)
	if err != nil {
		log.Errorf("error in yaml %s", err)
		return err
	}

	// Unmarshal initConfig to a RawConfigMap
	rawInitConfig := integration.RawMap{}
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
	conf := make(integration.RawMap)
	conf["name"] = c.ModuleName
	conf["init_config"] = rawInitConfig
	conf["instances"] = []interface{}{rawInstances}

	// Convert the RawConfigMap to a Python dictionary
	kwargs, err := ToPython(&conf) // don't `DecRef` kwargs since we keep it around in c.config
	if err != nil {
		log.Errorf("Error parsing python check configuration: %v", err)
		return err
	}

	// try getting an instance with the new style api, without passing agentConfig
	instance, err := c.getInstance(nil, kwargs) // don't `DecRef` instance since we keep it around in c.instance
	if err != nil {
		log.Warnf("could not get a check instance with the new api: %s", err)
		log.Warn("trying to instantiate the check with the old api, passing agentConfig to the constructor")

		// try again, assuming the check is good but has still the old api
		// we pass initConfig but emit a deprecation notice
		allSettings := config.Datadog.AllSettings()
		agentConfig, err := ToPython(allSettings)
		defer agentConfig.DecRef()
		if err != nil {
			log.Errorf("could not convert agent configuration to python: %s", err)
			return fmt.Errorf("could not convert agent configuration to python: %s", err)
		}

		// Add new 'agentConfig' key to the dict...
		gstate := newStickyLock()
		key := python.PyString_FromString("agentConfig")
		defer key.DecRef()
		python.PyDict_SetItem(kwargs, key, agentConfig)
		gstate.unlock()

		// ...and retry to get an instance
		instance, err = c.getInstance(nil, kwargs)
		if err != nil {
			return fmt.Errorf("could not invoke python check constructor: %s", err)
		}

		log.Warnf("passing `agentConfig` to the constructor is deprecated, please use the `get_config` function from the 'datadog_agent' package (%s).", c.ModuleName)
	}
	log.Debugf("python check configure done %s", c.ModuleName)

	// The Check ID is set in Python so that the python check
	// can use it afterwards to submit to the proper sender in the aggregator
	pyID := python.PyString_FromString(string(c.ID()))
	defer pyID.DecRef()
	instance.SetAttrString("check_id", pyID)

	c.instance = instance
	c.config = kwargs

	return nil
}

// GetMetricStats returns the stats from the last run of the check
func (c *PythonCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

// Interval returns the scheduling time for the check
func (c *PythonCheck) Interval() time.Duration {
	return c.interval
}

// ID returns the ID of the check
func (c *PythonCheck) ID() check.ID {
	return c.id
}

// pythonCheckFinalizer is a finalizer that decreases the reference count on the PyObject refs owned
// by the PythonCheck.
func pythonCheckFinalizer(c *PythonCheck) {
	// Run in a separate goroutine because acquiring the python lock might take some time,
	// and we're in a finalizer
	go func(c *PythonCheck) {
		glock := newStickyLock() // acquire lock to call DecRef
		defer glock.unlock()
		c.class.DecRef()
		if c.instance != nil {
			c.instance.DecRef()
		}
		if c.config != nil {
			c.config.DecRef()
		}
	}(c)
}
