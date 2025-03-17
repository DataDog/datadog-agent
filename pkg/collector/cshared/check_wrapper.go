// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cshared

/*
#cgo CFLAGS: -I./include

#include <stdlib.h>
#include "check_wrapper.h"
#include "sender.h"

char *_c_check_wrapper_run(c_check_wrapper_t *wrapper) {
	return wrapper->run(wrapper->handle);
}

void _c_check_wrapper_stop(c_check_wrapper_t *wrapper) {
	wrapper->stop(wrapper->handle);
}

void _c_check_wrapper_cancel(c_check_wrapper_t *wrapper) {
	wrapper->cancel(wrapper->handle);
}

char *_c_check_wrapper_to_string(c_check_wrapper_t *wrapper) {
	return wrapper->to_string(wrapper->handle);
}

char *_c_check_wrapper_loader(c_check_wrapper_t *wrapper) {
	return wrapper->loader(wrapper->handle);
}

char *_c_check_wrapper_configure(c_check_wrapper_t *wrapper, sender_manager_t *senderManager, uint64_t integrationConfigDigest, char *config, char *initConfig, char *source) {
	return wrapper->configure(wrapper->handle, senderManager, integrationConfigDigest, config, initConfig, source);
}

int64_t _c_check_wrapper_interval(c_check_wrapper_t *wrapper) {
	return wrapper->interval(wrapper->handle);
}

char *_c_check_wrapper_id(c_check_wrapper_t *wrapper) {
	return wrapper->id(wrapper->handle);
}

char *_c_check_wrapper_version(c_check_wrapper_t *wrapper) {
	return wrapper->version(wrapper->handle);
}

char *_c_check_wrapper_configSource(c_check_wrapper_t *wrapper) {
	return wrapper->configSource(wrapper->handle);
}

bool _c_check_wrapper_isTelemetryEnabled(c_check_wrapper_t *wrapper) {
	return wrapper->isTelemetryEnabled(wrapper->handle);
}

char *_c_check_wrapper_initConfig(c_check_wrapper_t *wrapper) {
	return wrapper->initConfig(wrapper->handle);
}

char *_c_check_wrapper_instanceConfig(c_check_wrapper_t *wrapper) {
	return wrapper->instanceConfig(wrapper->handle);
}

bool _c_check_wrapper_isHASupported(c_check_wrapper_t *wrapper) {
	return wrapper->isHASupported(wrapper->handle);
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)

var _ check.Check = (*cSharedCheck)(nil)

var checkPinner runtime.Pinner

type cSharedCheck struct {
	cWrapper *C.c_check_wrapper_t
}

func (c *cSharedCheck) Run() error {
	err := C._c_check_wrapper_run(c.cWrapper)
	defer C.free(unsafe.Pointer(err))
	if err != nil {
		return fmt.Errorf("cshared.check: could not run check %s: %s", c, C.GoString(err))
	}
	return nil
}

func (c *cSharedCheck) Stop() {
	C._c_check_wrapper_stop(c.cWrapper)
}

func (c *cSharedCheck) Cancel() {
	C._c_check_wrapper_cancel(c.cWrapper)
}

func (c *cSharedCheck) String() string {
	s := C._c_check_wrapper_to_string(c.cWrapper)
	defer C.free(unsafe.Pointer(s))
	return C.GoString(s)
}

func (c *cSharedCheck) Loader() string {
	s := C._c_check_wrapper_loader(c.cWrapper)
	defer C.free(unsafe.Pointer(s))
	return C.GoString(s)
}

func (c *cSharedCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, initConfig integration.Data, instanceConfig integration.Data, source string) error {
	cSenderManager := newCSenderManager(senderManager)

	initConfigC := C.CString(string(initConfig))
	instanceConfigC := C.CString(string(instanceConfig))
	sourceC := C.CString(source)
	defer C.free(unsafe.Pointer(initConfigC))
	defer C.free(unsafe.Pointer(instanceConfigC))
	defer C.free(unsafe.Pointer(sourceC))

	err := C._c_check_wrapper_configure(c.cWrapper, cSenderManager, C.uint64_t(integrationConfigDigest), initConfigC, instanceConfigC, sourceC)
	defer C.free(unsafe.Pointer(err))
	if err != nil {
		return fmt.Errorf("cshared.check: could not configure check %s: %s", c, C.GoString(err))
	}
	return nil
}

func (c *cSharedCheck) Interval() time.Duration {
	return time.Duration(C._c_check_wrapper_interval(c.cWrapper))
}

func (c *cSharedCheck) ID() checkid.ID {
	id := C._c_check_wrapper_id(c.cWrapper)
	defer C.free(unsafe.Pointer(id))
	return checkid.ID(C.GoString(id))
}

func (c *cSharedCheck) Version() string {
	version := C._c_check_wrapper_version(c.cWrapper)
	defer C.free(unsafe.Pointer(version))
	return C.GoString(version)
}

func (c *cSharedCheck) ConfigSource() string {
	source := C._c_check_wrapper_configSource(c.cWrapper)
	defer C.free(unsafe.Pointer(source))
	return C.GoString(source)
}

func (c *cSharedCheck) IsTelemetryEnabled() bool {
	return bool(C._c_check_wrapper_isTelemetryEnabled(c.cWrapper))
}

func (c *cSharedCheck) InitConfig() string {
	initConfig := C._c_check_wrapper_initConfig(c.cWrapper)
	defer C.free(unsafe.Pointer(initConfig))
	return C.GoString(initConfig)
}

func (c *cSharedCheck) InstanceConfig() string {
	instanceConfig := C._c_check_wrapper_instanceConfig(c.cWrapper)
	defer C.free(unsafe.Pointer(instanceConfig))
	return C.GoString(instanceConfig)
}

func (c *cSharedCheck) IsHASupported() bool {
	return bool(C._c_check_wrapper_isHASupported(c.cWrapper))
}

func (c *cSharedCheck) GetDiagnoses() ([]diagnosis.Diagnosis, error) {
	//TODO
	return nil, nil
}

func (c *cSharedCheck) GetSenderStats() (stats.SenderStats, error) {
	//TODO
	return stats.SenderStats{}, nil
}

func (c *cSharedCheck) GetWarnings() []error {
	//TODO
	return nil
}
