// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cshared

/*
#cgo CFLAGS: -I../include
#include <stdlib.h>
#include <stdbool.h>
#include <stdint.h>
#include "sender.h"
*/
import "C"

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checktypes "github.com/DataDog/datadog-agent/pkg/collector/check/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//export _call_check_run
func _call_check_run(handle unsafe.Pointer) *C.char {
	log.Debug("c-shared check run")
	check := *(*checktypes.Check)(handle)
	err := check.Run()
	if err != nil {
		return C.CString(err.Error())
	}
	return nil
}

//export _call_check_stop
func _call_check_stop(handle unsafe.Pointer) {
	log.Debug("c-shared check stop")
	check := *(*checktypes.Check)(handle)
	check.Stop()
}

//export _call_check_cancel
func _call_check_cancel(handle unsafe.Pointer) {
	log.Debug("c-shared check cancel")
	check := *(*checktypes.Check)(handle)
	check.Cancel()
}

//export _call_check_to_string
func _call_check_to_string(handle unsafe.Pointer) *C.char {
	log.Debug("c-shared check to_string")
	check := *(*checktypes.Check)(handle)
	return C.CString(check.String())
}

//export _call_check_loader
func _call_check_loader(handle unsafe.Pointer) *C.char {
	log.Debug("c-shared check loader")
	check := *(*checktypes.Check)(handle)
	loader := check.Loader()
	return C.CString(loader)
}

//export _call_check_configure
func _call_check_configure(handle unsafe.Pointer, senderManager *C.sender_manager_t, integrationConfigDigest C.uint64_t, config *C.char, initConfig *C.char, source *C.char) *C.char {
	log.Debug("c-shared check configure")
	check := *(*checktypes.Check)(handle)

	cSharedSenderManager := newCSharedSenderManager(senderManager)

	ret := check.Configure(cSharedSenderManager, uint64(integrationConfigDigest), integration.Data(C.GoString(config)), integration.Data(C.GoString(initConfig)), C.GoString(source))
	if ret != nil {
		return C.CString(ret.Error())
	}
	return nil
}

//export _call_check_interval
func _call_check_interval(handle unsafe.Pointer) C.int64_t {
	log.Debug("c-shared check interval")
	check := *(*checktypes.Check)(handle)
	return C.int64_t(check.Interval())
}

//export _call_check_id
func _call_check_id(handle unsafe.Pointer) *C.char {
	log.Debug("c-shared check id")
	check := *(*checktypes.Check)(handle)
	return C.CString(string(check.ID()))
}

//export _call_check_version
func _call_check_version(handle unsafe.Pointer) *C.char {
	log.Debug("c-shared check version")
	check := *(*checktypes.Check)(handle)
	return C.CString(check.Version())
}

//export _call_check_configSource
func _call_check_configSource(handle unsafe.Pointer) *C.char {
	log.Debug("c-shared check configSource")
	check := *(*checktypes.Check)(handle)
	return C.CString(check.ConfigSource())
}

//export _call_check_isTelemetryEnabled
func _call_check_isTelemetryEnabled(handle unsafe.Pointer) C.bool {
	log.Debug("c-shared check isTelemetryEnabled")
	check := *(*checktypes.Check)(handle)
	return C.bool(check.IsTelemetryEnabled())
}

//export _call_check_initConfig
func _call_check_initConfig(handle unsafe.Pointer) *C.char {
	log.Debug("c-shared check initConfig")
	check := *(*checktypes.Check)(handle)
	return C.CString(check.InitConfig())
}

//export _call_check_instanceConfig
func _call_check_instanceConfig(handle unsafe.Pointer) *C.char {
	log.Debug("c-shared check instanceConfig")
	check := *(*checktypes.Check)(handle)
	return C.CString(check.InstanceConfig())
}

//export _call_check_isHASupported
func _call_check_isHASupported(handle unsafe.Pointer) C.bool {
	log.Debug("c-shared check isHASupported")
	check := *(*checktypes.Check)(handle)
	return C.bool(check.IsHASupported())
}
