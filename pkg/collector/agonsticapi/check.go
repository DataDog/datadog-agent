// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agonsticapi

/*
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <dlfcn.h>
#include "loader.h"

extern void close_library(void *handle, const char **error)
{
	// Load the shared library
	if (!handle) {
		*error = strdup("Error loading library");
		return;
	}

	int result = dlclose(handle);
	if (result > 0) {
	 	*error = strdup("Error closing the library");
		return;
	}
}

extern Result* run_agnostic_check(void *handle, const char *id, const char **error)
{
	// Load the shared library
	if (!handle) {
		*error = strdup("Error loading library");
		return NULL;
	}

	// Clear any previous dlerror
  dlerror();

	// Load the function symbol
	Result* (*run)(const char*);
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
  Result *result = run(id);

	return result;
}

extern void free_result(void *handle, Result *result, const char **error)
{

	// Load the shared library
	if (!handle) {
		*error = strdup("Error loading library");
		return;
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
		return;
	}


	// Call the function
  freeResult(result);
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/agonsticapi/Integrations"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

type agnosticCheck struct {
	sendermanager     sender.SenderManager
	tagger            tagger.Component
	name              string
	libHandle         unsafe.Pointer
	id                checkid.ID
	initConfig        integration.Data
	instanceConfig    integration.Data
	configInitWritten bool
}

func newCheck(sendermanager sender.SenderManager, tagger tagger.Component, name string, lib unsafe.Pointer) (check.Check, error) {
	return &agnosticCheck{
		sendermanager: sendermanager,
		tagger:        tagger,
		name:          name,
		libHandle:     lib,
	}, nil
}

func (c *agnosticCheck) Run() error {
	var cErr *C.char

	idCStr := C.CString(string(c.id))
	defer C.free(unsafe.Pointer(idCStr))

	result := C.run_agnostic_check(c.libHandle, idCStr, &cErr)

	if cErr != nil {
		errorString := C.GoString(cErr)
		defer C.free(unsafe.Pointer(cErr))

		errMsg := fmt.Sprintf("failed to execute `run`  function on library %s: err %s", c.name, errorString)
		return errors.New(errMsg)
	}

	if result == nil {
		return fmt.Errorf("library %s execution did not returned any result", c.name)
	}

	// Extract the FlatBuffer bytes from the Result
	bufferData := C.GoBytes(unsafe.Pointer(result.data), result.len)

	// Deserialize the FlatBuffer payload
	payload := Integrations.GetRootAsPayload(bufferData, 0)

	for i := 0; i < payload.MetricsLength(); i++ {
		var metric Integrations.Metric
		if payload.Metrics(&metric, i) {
			name := string(metric.Name())
			metrciType := string(metric.Type())
			value := metric.Value()
			hostn, _ := hostname.Get(context.TODO())
			tags := []string{}

			for i := range metric.TagsLength() {
				tags = append(tags, string(metric.Tags(i)))
			}

			// We need to figure out what tags we need to fetch from the Agent
			extraTags, err := c.tagger.GlobalTags(types.LowCardinality)
			if err != nil {
				fmt.Printf("failed to get global tags: %s\n", err.Error())
			} else {
				tags = append(tags, extraTags...)
			}

			fmt.Printf("Metric: %s = %f (type: %s, tags: %v)\n", name, value, metrciType, tags)
			sender, err := c.sendermanager.GetSender(c.id)
			if err != nil {
				return fmt.Errorf("failed to get sender manager for shared library check %s: %v", c.name, err)
			}

			switch metrciType {
			case "gauge":
				sender.Gauge(name, value, hostn, tags)
			case "rate":
				sender.Rate(name, value, hostn, tags)
			case "count":
				sender.Count(name, value, hostn, tags)
			case "monotonic_count":
				sender.MonotonicCountWithFlushFirstValue(name, value, hostn, tags, false)
			case "counter":
				sender.Counter(name, value, hostn, tags)
			case "histogram":
				sender.Histogram(name, value, hostn, tags)
			case "historate":
				sender.Historate(name, value, hostn, tags)
			}
		}
	}

	C.free_result(c.libHandle, result, &cErr)
	if cErr != nil {
		errorString := C.GoString(cErr)
		defer C.free(unsafe.Pointer(cErr))
		// Log the error but don't fail the check since we already got the data
		fmt.Printf("Warning: failed to free result from library %s: %s\n", c.name, errorString)
	}

	return nil
}

func (c *agnosticCheck) Stop() {}

func (c *agnosticCheck) Cancel() {
	var cErr *C.char

	C.close_library(c.libHandle, &cErr)

	if cErr != nil {
		errorString := C.GoString(cErr)
		defer C.free(unsafe.Pointer(cErr))

		fmt.Printf("Warning: failed to cancel the shared library check %s: %s\n", c.name, errorString)
	}
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
	return string(c.initConfig)
}

func (c *agnosticCheck) InstanceConfig() string {
	return string(c.instanceConfig)
}

func (c *agnosticCheck) GetWarnings() []error {
	return []error{}
}

func (c *agnosticCheck) Configure(_senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, _source string) error {
	// Generate check ID
	c.id = checkid.BuildID(c.String(), integrationConfigDigest, data, initConfig)
	c.initConfig = initConfig
	c.instanceConfig = data

	// Write configuration to file once
	if !c.configInitWritten {
		if err := c.writeConfigToFile(initConfig, "init"); err != nil {
			return fmt.Errorf("failed to write init configuration file: %v", err)
		}
		c.configInitWritten = true
	}

	if err := c.writeConfigToFile(data, fmt.Sprintf("%s_instance", c.id)); err != nil {
		return fmt.Errorf("failed to write init configuration file: %v", err)
	}

	return nil
}

// writeConfigToFile writes the configuration as a FlatBuffer to a file
func (c *agnosticCheck) writeConfigToFile(data integration.Data, fileName string) error {
	builder := flatbuffers.NewBuilder(1024)

	// Create the byte vector first, before starting the object
	configValue := builder.CreateByteVector(data)

	// Now build the Configuration object
	Integrations.ConfigurationStart(builder)
	Integrations.ConfigurationAddValue(builder, configValue)
	conf := Integrations.ConfigurationEnd(builder)
	builder.Finish(conf)
	buf := builder.FinishedBytes()

	// Create a configuration file path in a system-accessible location
	configDir := filepath.Join("/tmp/datadog-agent-checks", c.name)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	configFile := filepath.Join(configDir, fmt.Sprintf("%s.bin", fileName))
	if err := os.WriteFile(configFile, buf, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	fmt.Printf("Configuration written to: %s\n", configFile)
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
