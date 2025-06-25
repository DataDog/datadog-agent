// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

/*
#include <stdlib.h>

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"
*/
import "C"

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
)

type SharedLibraryCheck struct {
	senderManager sender.SenderManager
	id            checkid.ID
	libName       string
	handle        unsafe.Pointer
}

func NewSharedLibraryCheck(senderManager sender.SenderManager, name string, handle unsafe.Pointer) (*SharedLibraryCheck, error) {
	check := &SharedLibraryCheck{
		senderManager: senderManager,
		libName:       name,
		handle:        handle,
	}

	return check, nil
}

func (c *SharedLibraryCheck) Run() error {
	var err *C.char

	cID := C.CString(string(c.ID()))
	defer C._free(unsafe.Pointer(cID))

	C.run_shared_library(cID, c.handle, &err)

	if err != nil {
		defer C._free(unsafe.Pointer(err))
		return fmt.Errorf("failed to run shared library check %s: %s", c.libName, C.GoString(err))
	}

	return nil
}

// check interface methods
func (c *SharedLibraryCheck) String() string {
	return c.libName
}

func (c *SharedLibraryCheck) Cancel() {
}

func (c *SharedLibraryCheck) ConfigSource() string {
	return ""
}

func (c *SharedLibraryCheck) Configure(_senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	c.id = checkid.BuildID(c.String(), integrationConfigDigest, data, initConfig)

	return nil
}

func (c *SharedLibraryCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return nil, nil
}

func (c *SharedLibraryCheck) GetSenderStats() (stats.SenderStats, error) {
	return stats.SenderStats{}, nil
}

func (c *SharedLibraryCheck) ID() checkid.ID {
	return checkid.ID(c.libName)
}

func (c *SharedLibraryCheck) InitConfig() string {
	return ""
}
func (c *SharedLibraryCheck) InstanceConfig() string {
	return ""
}
func (c *SharedLibraryCheck) IsHASupported() bool {
	return false
}

func (c *SharedLibraryCheck) IsTelemetryEnabled() bool {
	return false
}
func (c *SharedLibraryCheck) Loader() string {
	return SharedLibraryCheckLoaderName
}
func (c *SharedLibraryCheck) Interval() time.Duration {
	return 0 // Shared library checks typically do not have a defined interval
}

func (c *SharedLibraryCheck) Version() string {
	return "" // Versioning is not applicable for shared library checks
}
func (c *SharedLibraryCheck) GetWarnings() []error {
	return nil // No warnings to return for shared library checks
}

func (c *SharedLibraryCheck) Stop() {
}
