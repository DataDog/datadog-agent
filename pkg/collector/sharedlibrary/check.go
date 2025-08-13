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

void run_shared_library(char *instance, run_function_t *run_function, free_function_t *free_function, const char **error) {
	// verify pointers
    if (!run_function) {
        *error = strdup("pointer to shared library run function is null");
		return;
    }

	if (!free_function) {
        *error = strdup("pointer to shared library free function is null");
		return;
    }

    // run the shared library check and verify if an error has occurred
    char *run_error = run_function(instance, &submit_callbacks);
	if (run_error) {
		*error = strdup(run_error);
		free_function(run_error);
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
	senderManager sender.SenderManager
	id            checkid.ID
	instance      map[string]any
	interval      time.Duration
	libName       string
	libHandle     unsafe.Pointer     // pointer to the shared library
	runCb         *C.run_function_t  // run function callback
	freeCb        *C.free_function_t // free function callback
}

// NewSharedLibraryCheck conveniently creates a SharedLibraryCheck instance
func NewSharedLibraryCheck(senderManager sender.SenderManager, name string, libHandles C.shared_library_handles_t) (*SharedLibraryCheck, error) {
	check := &SharedLibraryCheck{
		senderManager: senderManager,
		interval:      defaults.DefaultCheckInterval,
		libName:       name,
		libHandle:     libHandles.lib,
		runCb:         libHandles.run,
		freeCb:        libHandles.free,
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
	C.run_shared_library(cInstance, c.runCb, c.freeCb, &cErr)
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

// String representation (for debug and logging)
func (c *SharedLibraryCheck) String() string {
	return c.libName
}

// Cancel is not implemented yet
func (c *SharedLibraryCheck) Cancel() {
}

// ConfigSource is not implemented yet
func (c *SharedLibraryCheck) ConfigSource() string {
	return ""
}

// Configure the shared library check from YAML data
func (c *SharedLibraryCheck) Configure(_senderManager sender.SenderManager, integrationConfigDigest uint64, instance integration.Data, initConfig integration.Data, _source string) error {
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

	return nil
}

// GetDiagnoses is not implemented yet
func (c *SharedLibraryCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return nil, nil
}

// GetSenderStats is not implemented yet
func (c *SharedLibraryCheck) GetSenderStats() (stats.SenderStats, error) {
	return stats.SenderStats{}, nil
}

// ID returns the ID of the check
func (c *SharedLibraryCheck) ID() checkid.ID {
	return checkid.ID(c.id)
}

// InitConfig is not implemented yet
func (c *SharedLibraryCheck) InitConfig() string {
	return ""
}

// InstanceConfig is not implemented yet
func (c *SharedLibraryCheck) InstanceConfig() string {
	return ""
}

// IsHASupported is not implemented yet
func (c *SharedLibraryCheck) IsHASupported() bool {
	return false
}

// IsTelemetryEnabled is not implemented yet
func (c *SharedLibraryCheck) IsTelemetryEnabled() bool {
	return false
}

// Loader returns the name of the loader
func (c *SharedLibraryCheck) Loader() string {
	return SharedLibraryCheckLoaderName
}

// Interval returns the interval between each check execution
func (c *SharedLibraryCheck) Interval() time.Duration {
	return c.interval
}

// Version is not implemented yet
func (c *SharedLibraryCheck) Version() string {
	return ""
}

// GetWarnings is not implemented yet
func (c *SharedLibraryCheck) GetWarnings() []error {
	return nil
}

// Stop is not implemented yet
func (c *SharedLibraryCheck) Stop() {
}
