package py

import (
	"errors"
	"fmt"
	"runtime"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/sbinet/go-python"

	log "github.com/cihub/seelog"
)

// #include <Python.h>
import "C"

// PythonCheck represents a Python check, implements `Check` interface
type PythonCheck struct {
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

// Configure the Python check from YAML data
func (c *PythonCheck) Configure(data check.ConfigData) error {
	// Unmarshal ConfigData to a RawConfigMap
	raw := check.ConfigRawMap{}
	err := yaml.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	// See if a collection interval was specified
	x, ok := raw["min_collection_interval"]
	if ok {
		// we should receive an int from the unmarshaller
		if intl, ok := x.(int); ok {
			// all good, convert to the right type
			c.interval = time.Duration(intl)
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
	conf["instances"] = []interface{}{raw}

	// Convert the RawConfigMap to a Python dictionary
	configDict, err := ToPythonDict(&conf)
	if err != nil {
		log.Errorf("Error parsing python check configuration: %v", err)
		return err
	}

	// invoke constructor
	emptyTuple := python.PyTuple_New(0)
	instance := c.Class.Call(emptyTuple, configDict)
	if instance == nil {
		// If the constructor is invalid we do not get a traceback but
		// an error in pvalue.
		_, pvalue, ptraceback := python.PyErr_Fetch()

		// The internal C pointer may be nill, the only way to check is
		// it to ask for the type, since the internal "ptr" is not
		// exposed.
		if pvalue.Type() != nil {
			log.Error(python.PyString_AsString(pvalue))
		}
		if ptraceback.Type() != nil {
			log.Error(python.PyString_AsString(ptraceback))
		}
		// python.PyErr_Print()
		return fmt.Errorf("could not invoke python check constructor")
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

// ID FIXME: this should return a real identifier
func (c *PythonCheck) ID() string {
	return c.String()
}
