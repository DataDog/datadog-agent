// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

/*
#include <stdlib.h>

#include "datadog_agent_rtloader.h"
#include "rtloader_types.h"
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
	libHandle     unsafe.Pointer       // handle to the shared library
	libRunHandle  *C.so_run_check_t    // handle to the function symbol that runs the check
	libFreeHandle *C.so_free_payload_t // handle to the function symbol that frees the check payload
}

func NewSharedLibraryCheck(senderManager sender.SenderManager, name string, libHandle unsafe.Pointer, libRunHandle *C.so_run_check_t, libFreeHandle *C.so_free_payload_t) (*SharedLibraryCheck, error) {
	check := &SharedLibraryCheck{
		senderManager: senderManager,
		libName:       name,
		libHandle:     libHandle,
		libRunHandle:  libRunHandle,
		libFreeHandle: libFreeHandle,
	}

	return check, nil
}

func (c *SharedLibraryCheck) Run() error {
	var err *C.char

	// the ID is used for sending the metrics, we need to know which check is running
	// to retrieve the correct sender
	cID := C.CString(string(c.ID()))
	defer C._free(unsafe.Pointer(cID))

	// execute the Run function of the shared library pointed by c.handle
	C.run_shared_library(cID, c.libRunHandle, c.libFreeHandle, &err)
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
	return 15 * time.Second
}

func (c *SharedLibraryCheck) Version() string {
	return ""
}
func (c *SharedLibraryCheck) GetWarnings() []error {
	return nil
}

func (c *SharedLibraryCheck) Stop() {
}
