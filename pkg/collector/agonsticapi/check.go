// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agonsticapi

/*
#include <stdio.h>
#include <stdlib.h>
#include <dlfcn.h>
#include "executor.h"

extern void close_library(void *handle)
{
	dlclose(handle);
}


extern Result* run_check(void *handle, const char **error)
{
	// Load the shared library
	if (!handle) {
		*error = strdup("Error loading library");
		return NULL;
	}

	// Clear any previous dlerror
  dlerror();

	// Load the function symbol
	Result* (*run)();
  *(void **)(&run) = dlsym(handle, "Run");

	// Check for errors
	char *dl_error = dlerror();
	if (dl_error) {
			size_t len = strlen("Error loading symbol 'Run': ") + strlen(dl_error) + 1;
			char *formatted_error = malloc(len);
			snprintf(formatted_error, len, "Error loading symbol 'Run': %s", dl_error);
			*error = formatted_error;
			return NULL;
	}


	// Call the function and get the result
  Result *result = run();

	return result;
}

extern void free_result(void *handle, Result *result, const char **error)
{

	// Load the shared library
	if (!handle) {
		*error = strdup("Error loading library");
		return NULL;
	}

	// Clear any previous dlerror
  dlerror();

	// Load the function symbol
	void (*freeResult)(Result*);
  *(void **)(&freeResult) = dlsym(handle, "FreeResult");

	// Check for errors
	char *dl_error = dlerror();
	if (dl_error) {
		size_t len = strlen("Error loading symbol 'FreeResult': ") + strlen(dl_error) + 1;
		char *formatted_error = malloc(len);
		snprintf(formatted_error, len, "Error loading symbol 'FreeResult': %s", dl_error);
		*error = formatted_error;
		return NULL;
	}


	// Call the function
  freeResult(result);
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
)

type Metric struct {
	Type  string   `json:"type"`
	Name  string   `json:"name"`
	Value uint64   `json:"value"`
	Tags  []string `json:"tags"`
}

type Payload struct {
	Metrics []Metric `json:"metrics"`
}

type agnosticCheck struct {
	sendermanager sender.SenderManager
	name          string
	libHandle     unsafe.Pointer
	id            checkid.ID
}

func NewCheck(sendermanager sender.SenderManager, name string, lib unsafe.Pointer) (check.Check, error) {
	return &agnosticCheck{
		sendermanager: sendermanager,
		name:          name,
		libHandle:     lib,
	}, nil
}

func (c *agnosticCheck) Run() error {
	var cErr *C.char

	result := C.run_check(c.libHandle, &cErr)

	if cErr != nil {
		errorString := C.GoString(cErr)
		defer C.free(unsafe.Pointer(cErr))

		// error message should not be too verbose, to keep the logs clean
		errMsg := fmt.Sprintf("failed to execute `run`  function on library %s: err %s", c.name, errorString)
		return errors.New(errMsg)
	}

	if result == nil {
		return errors.New(fmt.Sprintf("library %s execution did not return any result", c.name))
	}

	// Extract the JSON string from the Result using the len attribute
	jsonString := C.GoStringN(unsafe.Pointer(result.message), result.len)

	// Parse the JSON into our struct
	var payload Payload
	if err := json.Unmarshal([]byte(jsonString), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal JSON from library %s: %v", c.name, err)
	}

	// Free the result from the shared library
	C.free_result(c.libHandle, result, &cErr)
	if cErr != nil {
		errorString := C.GoString(cErr)
		defer C.free(unsafe.Pointer(cErr))
		// Log the error but don't fail the check since we already got the data
		fmt.Printf("Warning: failed to free result from library %s: %s\n", c.name, errorString)
	}

	// TODO: Process the metrics in the payload (send to aggregator, etc.)
	fmt.Printf("Received %d metrics from library %s\n", len(payload.Metrics), c.name)
	for _, metric := range payload.Metrics {
		fmt.Printf("Metric: %s = %d (type: %s, tags: %v)\n", metric.Name, metric.Value, metric.Type, metric.Tags)
	}

	return nil
}

func (c *agnosticCheck) Stop() {}

func (c *agnosticCheck) Cancel() {
}

func (c *agnosticCheck) String() string {
	return c.name
}

func (c *agnosticCheck) Version() string {
	return ""
}

func (c *agnosticCheck) IsTelemetryEnabled() bool {
	return false
}

func (c *agnosticCheck) ConfigSource() string {
	return ""
}

func (*agnosticCheck) Loader() string {
	return loaderName
}

func (c *agnosticCheck) InitConfig() string {
	return ""
}

func (c *agnosticCheck) InstanceConfig() string {
	return ""
}

func (c *agnosticCheck) GetWarnings() []error {
	return []error{}
}

func (c *agnosticCheck) Configure(_senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	// Generate check ID
	c.id = checkid.BuildID(c.String(), integrationConfigDigest, data, initConfig)

	return nil
}

func (c *agnosticCheck) GetSenderStats() (stats.SenderStats, error) {
	return stats.SenderStats{}, nil
}

func (c *agnosticCheck) Interval() time.Duration {
	return time.Hour
}

func (c *agnosticCheck) ID() checkid.ID {
	return c.id
}

func (c *agnosticCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return []diagnose.Diagnosis{}, nil
}

func (c *agnosticCheck) IsHASupported() bool {
	return false
}
