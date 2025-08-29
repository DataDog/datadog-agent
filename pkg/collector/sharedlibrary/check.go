// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

/*
#include <stdlib.h>
#include <stdio.h>
#include <stdbool.h>
#include <string.h>

#include "shared_library_types.h"

void SubmitMetricSo(char *, metric_type_t, char *, double, char **, char *, bool);
void SubmitServiceCheckSo(char *, char *, int, char **, char *, char *);
void SubmitEventSo(char *, event_t *);
void SubmitHistogramBucketSo(char *, char *, long long, float, float, int, char *, char **, bool);
void SubmitEventPlatformEventSo(char *, char *, int, char *);

static const submit_callbacks_t submit_callbacks = {
	SubmitMetricSo,
	SubmitServiceCheckSo,
	SubmitEventSo,
	SubmitHistogramBucketSo,
	SubmitEventPlatformEventSo,
};

void run_shared_library(char *instance, shared_library_handles_t lib_handles, const char **error) {
	// verify pointers
    if (!lib_handles.run) {
        *error = strdup("pointer to shared library run function is null");
		return;
    }

	if (!lib_handles.free) {
        *error = strdup("pointer to shared library free function is null");
		return;
    }

    // run the shared library check and verify if an error has occurred
    char *run_error = (lib_handles.run)(instance, &submit_callbacks);
	if (run_error) {
		*error = strdup(run_error);
		(lib_handles.free)(run_error);
	}
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"time"
	"unsafe"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SharedLibraryCheck represents a shared library check, implements `Check` interface
//
//nolint:revive
type SharedLibraryCheck struct {
	senderManager  sender.SenderManager
	id             checkid.ID
	version        string
	instance       map[string]any
	interval       time.Duration
	libName        string
	libHandles     C.shared_library_handles_t // store library file and symbols pointers
	source         string
	telemetry      bool // whether or not the telemetry is enabled for this check
	initConfig     string
	instanceConfig string
}

// NewSharedLibraryCheck conveniently creates a SharedLibraryCheck instance
func NewSharedLibraryCheck(senderManager sender.SenderManager, name string, libHandles C.shared_library_handles_t) (*SharedLibraryCheck, error) {
	check := &SharedLibraryCheck{
		senderManager: senderManager,
		interval:      defaults.DefaultCheckInterval,
		libName:       name,
		libHandles:    libHandles,
	}

	return check, nil
}

// Run a shared library check
func (c *SharedLibraryCheck) Run() error {
	var cErr *C.char

	// aggregate check parameters into a json
	jsonInstance, err := json.Marshal(c.instance)
	if err != nil {
		return err
	}

	cInstance := C.CString(string(jsonInstance))
	defer C.free(unsafe.Pointer(cInstance))

	// execute the check with the symbol retrieved earlier
	C.run_shared_library(cInstance, c.libHandles, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return fmt.Errorf("Failed to run shared library check %s: %s", c.libName, C.GoString(cErr))
	}

	s, err := c.senderManager.GetSender(c.ID())
	if err != nil {
		return fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	s.Commit()

	return nil
}

// Stop does nothing
func (c *SharedLibraryCheck) Stop() {}

// Cancel does nothing
func (c *SharedLibraryCheck) Cancel() {}

// String representation (for debug and logging)
func (c *SharedLibraryCheck) String() string {
	return c.libName
}

// Version is always an empty string
func (c *SharedLibraryCheck) Version() string {
	return c.version
}

// IsTelemetryEnabled is not enabled
func (c *SharedLibraryCheck) IsTelemetryEnabled() bool {
	return c.telemetry
}

// ConfigSource returns the source of the configuration for this check
func (c *SharedLibraryCheck) ConfigSource() string {
	return c.source
}

// Loader returns the check loader
func (c *SharedLibraryCheck) Loader() string {
	return SharedLibraryCheckLoaderName
}

// InitConfig returns the init_config configuration for the check
func (c *SharedLibraryCheck) InitConfig() string {
	return c.initConfig
}

// InstanceConfig returns the instance configuration for the check.
func (c *SharedLibraryCheck) InstanceConfig() string {
	return c.instanceConfig
}

// GetWarnings returns nothing
func (c *SharedLibraryCheck) GetWarnings() []error {
	return []error{}
}

// Configure the shared library check from YAML data
func (c *SharedLibraryCheck) Configure(_senderManager sender.SenderManager, integrationConfigDigest uint64, instance integration.Data, initConfig integration.Data, source string) error {
	c.id = checkid.BuildID(c.String(), integrationConfigDigest, instance, initConfig)

	commonOptions := integration.CommonInstanceConfig{}
	if err := yaml.Unmarshal(instance, &commonOptions); err != nil {
		log.Errorf("invalid instance section for check %s: %s", string(c.id), err)
		return err
	}

	// See if a collection interval was specified
	if commonOptions.MinCollectionInterval > 0 {
		c.interval = time.Duration(commonOptions.MinCollectionInterval) * time.Second
	}

	if err := yaml.Unmarshal(instance, &c.instance); err != nil {
		log.Errorf("invalid instance section for check %s: %s", string(c.id), err)
		return err
	}

	// common check parameters
	c.instance["check_id"] = c.id
	if _, ok := c.instance["tags"]; !ok { // check if there's no `tags` key
		// create empty list in case that there's no tags field
		c.instance["tags"] = make([]string, 0)
	}

	// configuration info
	c.source = source
	c.initConfig = string(initConfig)
	c.instanceConfig = string(instance)

	return nil
}

// GetSenderStats returns the stats from the last run of the check
func (c *SharedLibraryCheck) GetSenderStats() (stats.SenderStats, error) {
	sender, err := c.senderManager.GetSender(c.ID())
	if err != nil {
		return stats.SenderStats{}, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetSenderStats(), nil
}

// Interval returns the interval between each check execution
func (c *SharedLibraryCheck) Interval() time.Duration {
	return c.interval
}

// ID returns the ID of the check
func (c *SharedLibraryCheck) ID() checkid.ID {
	return checkid.ID(c.id)
}

// GetDiagnoses returns nothing
func (c *SharedLibraryCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return nil, nil
}

// IsHASupported does not apply to shared library checks
func (c *SharedLibraryCheck) IsHASupported() bool {
	return false
}
