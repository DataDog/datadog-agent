// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

/*
#include <stdlib.h>

#include "shared_library.h"

void SubmitMetricSo(char *, metric_type_t, char *, double, char **, char *, bool);
void SubmitServiceCheckSo(char *, char *, int, char **, char *, char *);
void SubmitEventSo(char *, event_t *);
void SubmitHistogramBucketSo(char *, char *, long long, float, float, int, char *, char **, bool);
void SubmitEventPlatformEventSo(char *, char *, int, char *);

// the callbacks are aggregated in this file as it's the only one which uses it
static const aggregator_t aggregator = {
	SubmitMetricSo,
	SubmitServiceCheckSo,
	SubmitEventSo,
	SubmitHistogramBucketSo,
	SubmitEventPlatformEventSo,
};

static const aggregator_t *get_aggregator() {
	return &aggregator;
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
	senderManager  sender.SenderManager
	id             checkid.ID
	version        string
	interval       time.Duration
	libName        string
	libHandles     C.handles_t // store library file and symbols pointers
	source         string
	telemetry      bool   // whether or not the telemetry is enabled for this check
	initConfig     string // json string of check common config
	instanceConfig string // json string of specific instance config
	cancelled      bool
}

// NewSharedLibraryCheck conveniently creates a SharedLibraryCheck instance
func NewSharedLibraryCheck(senderManager sender.SenderManager, name string, libHandles C.handles_t) (*SharedLibraryCheck, error) {
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
	return c.runCheckImpl(true)
}

// runCheckImpl runs the check implementation with its Run symbol
// This function is created to allow passing the commitMetrics parameter (not possible due to the Check interface)
func (c *SharedLibraryCheck) runCheckImpl(commitMetrics bool) error {
	if c.cancelled {
		return fmt.Errorf("check %s is already cancelled", c.libName)
	}

	cID := C.CString(string(c.id))
	defer C.free(unsafe.Pointer(cID))

	cInitConfig := C.CString(c.initConfig)
	defer C.free(unsafe.Pointer(cInitConfig))

	cInstanceConfig := C.CString(c.instanceConfig)
	defer C.free(unsafe.Pointer(cInstanceConfig))

	// retrieve callbacks
	cAggregator := C.get_aggregator()

	var cErr *C.char

	// run check implementation by using the symbol handle
	C.run_shared_library(c.libHandles.run, cID, cInitConfig, cInstanceConfig, cAggregator, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return fmt.Errorf("Run failed: %s", C.GoString(cErr))
	}

	if commitMetrics {
		s, err := c.senderManager.GetSender(c.ID())
		if err != nil {
			return fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
		}
		s.Commit()
	}

	return nil
}

// Stop does nothing
func (c *SharedLibraryCheck) Stop() {}

// Cancel closes the associated shared library and unschedules the check
func (c *SharedLibraryCheck) Cancel() {
	var cErr *C.char

	C.close_shared_library(c.libHandles.lib, &cErr)

	c.cancelled = true

	// TODO: unschedule check
}

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
func (c *SharedLibraryCheck) Configure(_senderManager sender.SenderManager, integrationConfigDigest uint64, instanceConfig integration.Data, initConfig integration.Data, source string) error {
	c.id = checkid.BuildID(c.String(), integrationConfigDigest, instanceConfig, initConfig)

	commonOptions := integration.CommonInstanceConfig{}
	if err := yaml.Unmarshal(instanceConfig, &commonOptions); err != nil {
		log.Errorf("invalid instance section for check %s: %s", string(c.id), err)
		return err
	}

	// See if a collection interval was specified
	if commonOptions.MinCollectionInterval > 0 {
		c.interval = time.Duration(commonOptions.MinCollectionInterval) * time.Second
	}

	// configurations
	c.source = source

	c.initConfig = string(initConfig)
	c.instanceConfig = string(instanceConfig)

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
