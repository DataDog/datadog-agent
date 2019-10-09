// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python

package python

import (
	"unsafe"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"
*/
import (
	"C"
)

// GetVersion exposes the version of the agent to Python checks.
//export GetVersion
func GetVersion(agentVersion **C.char) {
	av, _ := version.Agent()
	// version will be free by rtloader when it's done with it
	*agentVersion = TrackedCString(av.GetNumber())
}

// GetHostname exposes the current hostname of the agent to Python checks.
//export GetHostname
func GetHostname(hostname **C.char) {
	goHostname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err)
		goHostname = ""
	}
	// hostname will be free by rtloader when it's done with it
	*hostname = TrackedCString(goHostname)
}

// GetClusterName exposes the current clustername (if it exists) of the agent to Python checks.
//export GetClusterName
func GetClusterName(clusterName **C.char) {
	goClusterName := clustername.GetClusterName()
	// clusterName will be free by rtloader when it's done with it
	*clusterName = TrackedCString(goClusterName)
}

// TracemallocEnabled exposes the tracemalloc configuration of the agent to Python checks.
//export TracemallocEnabled
func TracemallocEnabled() C.bool {
	return C.bool(config.Datadog.GetBool("tracemalloc_debug"))
}

// Headers returns a basic set of HTTP headers that can be used by clients in Python checks.
//export Headers
func Headers(yamlPayload **C.char) {
	h := util.HTTPHeaders()

	data, err := yaml.Marshal(h)
	if err != nil {
		log.Errorf("datadog_agent: could not Marshal headers: %s", err)
		*yamlPayload = nil
		return
	}
	// yamlPayload will be free by rtloader when it's done with it
	*yamlPayload = TrackedCString(string(data))
}

// GetConfig returns a value from the agent configuration.
// Indirectly used by the C function `get_config` that's mapped to `datadog_agent.get_config`.
//export GetConfig
func GetConfig(key *C.char, yamlPayload **C.char) {
	goKey := C.GoString(key)
	if !config.Datadog.IsSet(goKey) {
		*yamlPayload = nil
		return
	}

	value := config.Datadog.Get(goKey)
	data, err := yaml.Marshal(value)
	if err != nil {
		log.Errorf("could not convert configuration value '%v' to YAML: %s", value, err)
		*yamlPayload = nil
		return
	}
	// yaml Payload will be free by rtloader when it's done with it
	*yamlPayload = TrackedCString(string(data))
}

// LogMessage logs a message from python through the agent logger (see
// https://docs.python.org/2.7/library/logging.html#logging-levels)
//export LogMessage
func LogMessage(message *C.char, logLevel C.int) {
	goMsg := C.GoString(message)

	switch logLevel {
	case 50: // CRITICAL
		log.Critical(goMsg)
	case 40: // ERROR
		log.Error(goMsg)
	case 30: // WARNING
		log.Warn(goMsg)
	case 20: // INFO
		log.Info(goMsg)
	case 10: // DEBUG
		log.Debug(goMsg)
	// Custom log level defined in:
	// https://github.com/DataDog/integrations-core/blob/master/datadog_checks_base/datadog_checks/base/log.py
	case 7: // TRACE
		log.Trace(goMsg)
	default: // unknown log level
		log.Info(goMsg)
	}

	return
}

// SetExternalTags adds a set of tags for a given hostname to the External Host
// Tags metadata provider cache.
//export SetExternalTags
func SetExternalTags(hostname *C.char, sourceType *C.char, tags **C.char) {
	hname := C.GoString(hostname)
	stype := C.GoString(sourceType)
	tagsStrings := []string{}

	pStart := unsafe.Pointer(tags)
	size := unsafe.Sizeof(*tags)
	for i := 0; ; i++ {
		pTag := *(**C.char)(unsafe.Pointer(uintptr(pStart) + size*uintptr(i)))
		if pTag == nil {
			break
		}
		tag := C.GoString(pTag)
		tagsStrings = append(tagsStrings, tag)
	}

	externalhost.SetExternalTags(hname, stype, tagsStrings)
}

// SetCheckMetadata updates a metadata value for one check instance in the cache.
// Indirectly used by the C function `set_check_metadata` that's mapped to `datadog_agent.set_check_metadata`.
//export SetCheckMetadata
func SetCheckMetadata(checkID, name, value *C.char) {
	cid := C.GoString(checkID)
	key := C.GoString(name)
	val := C.GoString(value)

	inventories.SetCheckMetadata(cid, key, val)
}

// WritePersistentCache stores a value for one check instance
// Indirectly used by the C function `write_persistent_cache` that's mapped to `datadog_agent.write_persistent_cache`.
//export WritePersistentCache
func WritePersistentCache(key, value *C.char) {
	key_name := C.GoString(key)
	val := C.GoString(value)
	persistentcache.Write(key_name, val)
}

// ReadPersistentCache retrieves a value for one check instance
// Indirectly used by the C function `read_persistent_cache` that's mapped to `datadog_agent.read_persistent_cache`.
//export ReadPersistentCache
func ReadPersistentCache(key *C.char, value **C.char) {
	key_name := C.GoString(key)
	data, err = persistentcache.Read(key_name)
	if err != nil {
		log.Errorf("Failed to read cache %s: %s", key_name, err)
		*value = nil
		return
	}
	*value = TrackedCString(data)
}
