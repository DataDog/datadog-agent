// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

/*
#include <stdlib.h>
#include <stdio.h>
#include <stdbool.h>

#include "shared_library_types.h"

static cb_submit_metric_t cb_submit_metric;
static cb_submit_service_check_t cb_submit_service_check;
static cb_submit_event_t cb_submit_event;
static cb_submit_histogram_bucket_t cb_submit_histogram_bucket;
static cb_submit_event_platform_event_t cb_submit_event_platform_event;

void SubmitMetricSo(char *, metric_type_t, char *, double, char **, char *, bool);
void SubmitServiceCheckSo(char *, char *, int, char **, char *, char *);
void SubmitEventSo(char *, event_t *);
void SubmitHistogramBucketSo(char *, char *, long long, float, float, int, char *, char **, bool);
void SubmitEventPlatformEventSo(char *, char *, int, char *);

void set_submit_metric_cb(cb_submit_metric_t cb) {
	cb_submit_metric = cb;
}

void set_submit_service_check_cb(cb_submit_service_check_t cb) {
	cb_submit_service_check = cb;
}

void set_submit_event_cb(cb_submit_event_t cb) {
	cb_submit_event = cb;
}

void set_submit_histogram_bucket_cb(cb_submit_histogram_bucket_t cb) {
	cb_submit_histogram_bucket = cb;
}

void set_submit_event_platform_event_cb(cb_submit_event_platform_event_t cb) {
	cb_submit_event_platform_event = cb;
}

void set_callbacks(void) {
	set_submit_metric_cb(SubmitMetricSo);
	set_submit_service_check_cb(SubmitServiceCheckSo);
	set_submit_event_cb(SubmitEventSo);
	set_submit_histogram_bucket_cb(SubmitHistogramBucketSo);
	set_submit_event_platform_event_cb(SubmitEventPlatformEventSo);
}

void run_shared_library(char *check_id, run_shared_library_check_t *run_function, const char **error) {
	// verify the run function pointer
    if (!run_function) {
        *error = "Pointer to shared library run function is null";
		return;
    }

	check_config_t config = {
		check_id,
		cb_submit_metric,
		cb_submit_service_check,
		cb_submit_event,
		cb_submit_histogram_bucket,
		cb_submit_event_platform_event,
	};

	printf("Submit Metric: %p\n", config.cb_submit_metric);
	printf("Submit Service Check: %p\n", config.cb_submit_service_check);
	printf("Submit Event: %p\n", config.cb_submit_event);
	printf("Submit Histogram bucket: %p\n", config.cb_submit_histogram_bucket);
	printf("Submit Event Platform Event: %p\n", config.cb_submit_event_platform_event);

    // run the shared library check and check the returned payload`
    //run_function(&config);
	printf("check execution finished\n");
}
*/
import "C"

import (
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
	interval      time.Duration
	libName       string
	libPtr        unsafe.Pointer                // pointer to the shared library (unused in RTLoader because it only needs the symbols)
	libRunPtr     *C.run_shared_library_check_t // pointer to the function symbol that runs the check
}

// NewSharedLibraryCheck conveniently creates a SharedLibraryCheck instance
func NewSharedLibraryCheck(senderManager sender.SenderManager, name string, libPtr unsafe.Pointer, libRunPtr *C.run_shared_library_check_t) (*SharedLibraryCheck, error) {
	check := &SharedLibraryCheck{
		senderManager: senderManager,
		interval:      defaults.DefaultCheckInterval,
		libName:       name,
		libPtr:        libPtr,
		libRunPtr:     libRunPtr,
	}

	return check, nil
}

// Run a shared library check
func (c *SharedLibraryCheck) Run() error {
	var err *C.char

	// the ID is used for sending the metrics, we need to know which check is running
	// to retrieve the correct sender
	cID := C.CString(string(c.ID()))
	defer C.free(unsafe.Pointer(cID))

	// execute the check with the symbol retrieved earlier
	//C.run_shared_library(cID, c.libRunPtr, &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return fmt.Errorf("failed to run shared library check %s: %s", c.libName, C.GoString(err))
	}

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
func (c *SharedLibraryCheck) Configure(_senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, _source string) error {
	c.id = checkid.BuildID(c.String(), integrationConfigDigest, data, initConfig)

	commonOptions := integration.CommonInstanceConfig{}
	if err := yaml.Unmarshal(data, &commonOptions); err != nil {
		log.Errorf("invalid instance section for check %s: %s", string(c.id), err)
		return err
	}

	// See if a collection interval was specified
	if commonOptions.MinCollectionInterval > 0 {
		c.interval = time.Duration(commonOptions.MinCollectionInterval) * time.Second
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
	// c.id is not the same as c.libName (it has an id after the name so the sender found by SubmitMetricRtLoader is a different one and metrics aren't submitted)
	return checkid.ID(c.libName)
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

// set_callbacks
func setCallbacks() {
	C.set_callbacks()
}
