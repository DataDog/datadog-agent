// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

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
	telemetry_utils "github.com/DataDog/datadog-agent/pkg/telemetry/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <stdlib.h>

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"

char *getStringAddr(char **array, unsigned int idx);
*/
import "C"

// PythonCheck represents a Python check, implements `Check` interface
type PythonCheck struct {
	id             check.ID
	version        string
	instance       *C.rtloader_pyobject_t
	class          *C.rtloader_pyobject_t
	ModuleName     string
	interval       time.Duration
	lastWarnings   []error
	source         string
	telemetry      bool // whether or not the telemetry is enabled for this check
	initConfig     string
	instanceConfig string
}

// NewPythonCheck conveniently creates a PythonCheck instance
func NewPythonCheck(name string, class *C.rtloader_pyobject_t) (*PythonCheck, error) {
	glock, err := newStickyLock()
	if err != nil {
		return nil, err
	}

	C.rtloader_incref(rtloader, class) // own the ref
	glock.unlock()

	pyCheck := &PythonCheck{
		ModuleName:   name,
		class:        class,
		interval:     defaults.DefaultCheckInterval,
		lastWarnings: []error{},
		telemetry:    telemetry_utils.IsCheckEnabled(name),
	}
	runtime.SetFinalizer(pyCheck, pythonCheckFinalizer)

	return pyCheck, nil
}

func (c *PythonCheck) runCheck(commitMetrics bool) error {
	// Lock the GIL and release it at the end of the run
	gstate, err := newStickyLock()
	if err != nil {
		return err
	}
	defer gstate.unlock()

	log.Debugf("Running python check %s (version: '%s', id: '%s')", c.ModuleName, c.version, c.id)

	cResult := C.run_check(rtloader, c.instance)
	if cResult == nil {
		if err := getRtLoaderError(); err != nil {
			return err
		}
		return fmt.Errorf("An error occurred while running python check %s", c.ModuleName)
	}
	defer C.rtloader_free(rtloader, unsafe.Pointer(cResult))

	if commitMetrics {
		s, err := aggregator.GetSender(c.ID())
		if err != nil {
			return fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
		}
		s.Commit()
	}

	// grab the warnings and add them to the struct
	c.lastWarnings = c.getPythonWarnings(gstate)

	checkErrStr := C.GoString(cResult)
	if checkErrStr == "" {
		return nil
	}
	return errors.New(checkErrStr)
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

// Cancel signals to a python check that he can free all internal resources and
// deregisters the sender
func (c *PythonCheck) Cancel() {
	gstate, err := newStickyLock()
	if err != nil {
		log.Warnf("failed to cancel check %s: %s", c.id, err)
		return
	}
	defer gstate.unlock()

	C.cancel_check(rtloader, c.instance)
	if err := getRtLoaderError(); err != nil {
		log.Warnf("failed to cancel check %s: %s", c.id, err)
	}
}

// String representation (for debug and logging)
func (c *PythonCheck) String() string {
	return c.ModuleName
}

// Version returns the version of the check if load from a python wheel
func (c *PythonCheck) Version() string {
	return c.version
}

// IsTelemetryEnabled returns if the telemetry is enabled for this check
func (c *PythonCheck) IsTelemetryEnabled() bool {
	return c.telemetry
}

// ConfigSource returns the source of the configuration for this check
func (c *PythonCheck) ConfigSource() string {
	return c.source
}

// InitConfig returns the init_config configuration for the check.
func (c *PythonCheck) InitConfig() string {
	return c.initConfig
}

// InstanceConfig returns the instance configuration for the check.
func (c *PythonCheck) InstanceConfig() string {
	return c.instanceConfig
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

	pyWarnings := C.get_checks_warnings(rtloader, c.instance)
	if pyWarnings == nil {
		if err := getRtLoaderError(); err != nil {
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
		C.rtloader_free(rtloader, unsafe.Pointer(warnPtr))
	}
	C.rtloader_free(rtloader, unsafe.Pointer(pyWarnings))

	return warnings
}

// Configure the Python check from YAML data
func (c *PythonCheck) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	// Generate check ID
	c.id = check.BuildID(c.String(), integrationConfigDigest, data, initConfig)

	commonGlobalOptions := integration.CommonGlobalConfig{}
	if err := yaml.Unmarshal(initConfig, &commonGlobalOptions); err != nil {
		log.Errorf("invalid init_config section for check %s: %s", string(c.id), err)
		return err
	}

	// Set service for this check
	if len(commonGlobalOptions.Service) > 0 {
		s, err := aggregator.GetSender(c.id)
		if err != nil {
			log.Errorf("failed to retrieve a sender for check %s: %s", string(c.id), err)
		} else {
			s.SetCheckService(commonGlobalOptions.Service)
		}
	}

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

	// Set configured service for this check, overriding the one possibly defined globally
	if len(commonOptions.Service) > 0 {
		s, err := aggregator.GetSender(c.id)
		if err != nil {
			log.Errorf("failed to retrieve a sender for check %s: %s", string(c.id), err)
		} else {
			s.SetCheckService(commonOptions.Service)
		}
	}

	cInitConfig := TrackedCString(string(initConfig))
	cInstance := TrackedCString(string(data))
	cCheckID := TrackedCString(string(c.id))
	cCheckName := TrackedCString(c.ModuleName)
	defer C._free(unsafe.Pointer(cInitConfig))
	defer C._free(unsafe.Pointer(cInstance))
	defer C._free(unsafe.Pointer(cCheckID))
	defer C._free(unsafe.Pointer(cCheckName))

	var check *C.rtloader_pyobject_t
	res := C.get_check(rtloader, c.class, cInitConfig, cInstance, cCheckID, cCheckName, &check)
	var rtLoaderError error
	if res == 0 {
		rtLoaderError = getRtLoaderError()
		log.Warnf("could not get a '%s' check instance with the new api: %s", c.ModuleName, rtLoaderError)
		log.Warn("trying to instantiate the check with the old api, passing agentConfig to the constructor")

		allSettings := config.Datadog.AllSettings()
		agentConfig, err := yaml.Marshal(allSettings)
		if err != nil {
			log.Errorf("error serializing agent config: %s", err)
			return err
		}
		cAgentConfig := TrackedCString(string(agentConfig))
		defer C._free(unsafe.Pointer(cAgentConfig))

		res := C.get_check_deprecated(rtloader, c.class, cInitConfig, cInstance, cAgentConfig, cCheckID, cCheckName, &check)
		if res == 0 {
			if rtLoaderError != nil {
				return fmt.Errorf("could not invoke '%s' python check constructor. New constructor API returned:\n%sDeprecated constructor API returned:\n%s", c.ModuleName, rtLoaderError, getRtLoaderError())
			}
			return fmt.Errorf("could not invoke '%s' python check constructor: %s", c.ModuleName, getRtLoaderError())
		}
		log.Warnf("passing `agentConfig` to the constructor is deprecated, please use the `get_config` function from the 'datadog_agent' package (%s).", c.ModuleName)
	}
	c.instance = check
	c.source = source

	// Add the possibly configured service as a tag for this check
	s, err := aggregator.GetSender(c.id)
	if err != nil {
		log.Errorf("failed to retrieve a sender for check %s: %s", string(c.id), err)
	} else {
		s.FinalizeCheckServiceTag()
	}

	c.initConfig = string(initConfig)
	c.instanceConfig = string(data)

	log.Debugf("python check configure done %s", c.ModuleName)
	return nil
}

// GetSenderStats returns the stats from the last run of the check
func (c *PythonCheck) GetSenderStats() (check.SenderStats, error) {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return check.SenderStats{}, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetSenderStats(), nil
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
		log.Debugf("Running finalizer for check %s", c.id)

		glock, err := newStickyLock() // acquire lock to call DecRef
		if err != nil {
			log.Warnf("Could not finalize check %s: %s", c.id, err.Error())
			return
		}
		defer glock.unlock()

		C.rtloader_decref(rtloader, c.class)
		if c.instance != nil {
			C.rtloader_decref(rtloader, c.instance)
		}
	}(c)
}
