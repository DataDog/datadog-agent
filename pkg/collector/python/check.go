// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python

package python

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <stdlib.h>
#include <datadog_agent_six.h>

char *getStringAddr(char **array, unsigned int idx);
*/
import "C"

// PythonCheck represents a Python check, implements `Check` interface
type PythonCheck struct {
	id           check.ID
	version      string
	instance     *C.six_pyobject_t
	class        *C.six_pyobject_t
	ModuleName   string
	interval     time.Duration
	lastWarnings []error
}

// NewPythonCheck conveniently creates a PythonCheck instance
func NewPythonCheck(name string, class *C.six_pyobject_t) *PythonCheck {
	glock := newStickyLock()
	C.six_incref(six, class) // own the ref
	glock.unlock()
	pyCheck := &PythonCheck{
		ModuleName:   name,
		class:        class,
		interval:     defaults.DefaultCheckInterval,
		lastWarnings: []error{},
	}
	runtime.SetFinalizer(pyCheck, pythonCheckFinalizer)
	return pyCheck
}

func (c *PythonCheck) runCheck(commitMetrics bool) error {
	// Lock the GIL and release it at the end of the run
	gstate := newStickyLock()
	defer gstate.unlock()

	log.Debugf("Running python check %s %s", c.ModuleName, c.id)

	cResult := C.run_check(six, c.instance)
	if cResult == nil {
		if err := getSixError(); err != nil {
			return err
		}
		return fmt.Errorf("An error occurred while running python check %s", c.ModuleName)
	}
	defer C.six_free(six, unsafe.Pointer(cResult))

	if commitMetrics {
		s, err := aggregator.GetSender(c.ID())
		if err != nil {
			return fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
		}
		s.Commit()
	}

	// grab the warnings and add them to the struct
	c.lastWarnings = c.getPythonWarnings(gstate)

	err := C.GoString(cResult)
	if err == "" {
		return nil
	}
	return errors.New(err)
}

// Run a Python check
func (c *PythonCheck) Run() error {
	return c.runCheck(true)
}

// RunSimple runs a Python check without sending data to the aggregator
func (c *PythonCheck) RunSimple() error {
	return c.runCheck(false)
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
	This function is run with the GIL locked by runCheck
	**/

	pyWarnings := C.get_checks_warnings(six, c.instance)
	if pyWarnings == nil {
		if err := getSixError(); err != nil {
			log.Errorf("error while collecting python check's warnings: %s", err)
		}
		return nil
	}

	warnings := []error{}
	for i := 0; ; i++ {
		// Work around go vet raising issue about unsafe pointer
		warnPtr := C.getStringAddr(pyWarnings, C.uint(i))
		if warnPtr == nil {
			break
		}
		warn := C.GoString(warnPtr)
		warnings = append(warnings, errors.New(warn))
		C.six_free(six, unsafe.Pointer(warnPtr))
	}
	C.six_free(six, unsafe.Pointer(pyWarnings))

	return warnings
}

// Configure the Python check from YAML data
func (c *PythonCheck) Configure(data integration.Data, initConfig integration.Data) error {
	// Generate check ID
	c.id = check.Identify(c, data, initConfig)

	commonOptions := integration.CommonInstanceConfig{}
	if err := yaml.Unmarshal(data, &commonOptions); err != nil {
		log.Errorf("invalid instance section for check %s: %s", string(c.id), err)
		return err
	}

	// See if a collection interval was specified
	if commonOptions.MinCollectionInterval > 0 {
		c.interval = time.Duration(commonOptions.MinCollectionInterval) * time.Second
	}

	// Disable default hostname if specified
	if commonOptions.EmptyDefaultHostname {
		s, err := aggregator.GetSender(c.id)
		if err != nil {
			log.Errorf("failed to retrieve a sender for check %s: %s", string(c.id), err)
		} else {
			s.DisableDefaultHostname(true)
		}
	}

	cInitConfig := C.CString(string(initConfig))

	cInstance := C.CString(string(data))
	cCheckID := C.CString(string(c.id))
	cCheckName := C.CString(c.ModuleName)
	defer C.free(unsafe.Pointer(cInitConfig))
	defer C.free(unsafe.Pointer(cInstance))
	defer C.free(unsafe.Pointer(cCheckID))
	defer C.free(unsafe.Pointer(cCheckName))

	var check *C.six_pyobject_t
	res := C.get_check(six, c.class, cInitConfig, cInstance, cCheckID, cCheckName, &check)
	if res == 0 {
		log.Warnf("could not get a '%s' check instance with the new api: %s", c.ModuleName, getSixError())
		log.Warn("trying to instantiate the check with the old api, passing agentConfig to the constructor")

		allSettings := config.Datadog.AllSettings()
		agentConfig, err := yaml.Marshal(allSettings)
		if err != nil {
			log.Errorf("error serializing agent config: %s", err)
			return err
		}
		cAgentConfig := C.CString(string(agentConfig))
		defer C.free(unsafe.Pointer(cAgentConfig))

		res := C.get_check_deprecated(six, c.class, cInitConfig, cInstance, cAgentConfig, cCheckID, cCheckName, &check)
		if res == 0 {
			return fmt.Errorf("could not invoke '%s' python check constructor: %s", c.ModuleName, getSixError())
		}
		log.Warnf("passing `agentConfig` to the constructor is deprecated, please use the `get_config` function from the 'datadog_agent' package (%s).", c.ModuleName)
	}
	c.instance = check

	log.Debugf("python check configure done %s", c.ModuleName)
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
		C.six_decref(six, c.class)
		if c.instance != nil {
			C.six_decref(six, c.instance)
		}
	}(c)
}
