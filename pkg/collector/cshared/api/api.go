// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cshared exposes a C api for c-shared libraries
package cshared

/*
// this is to ease the cgo compilation but should eventually be removed
#cgo CFLAGS: -I../../../../rtloader/include

#include <stdlib.h>
#include <stdbool.h>
#include "rtloader_types.h"

// this one shouldn't be needed
//extern void MemoryTracker(void *, size_t, rtloader_mem_ops_t);
extern void LogMessage(char *, int);
extern void GetClusterName(char **);
extern void GetConfig(char*, char **);
extern void GetHostname(char **);
extern void GetHostTags(char **);
extern void GetVersion(char **);
extern void Headers(char **);
extern char * ReadPersistentCache(char *);
extern void SendLog(char *, char *);
extern void SetCheckMetadata(char *, char *, char *);
extern void SetExternalTags(char *, char *, char **);
extern void WritePersistentCache(char *, char *);
extern bool TracemallocEnabled();
extern char* ObfuscateSQL(char *, char *, char **);
extern char* ObfuscateSQLExecPlan(char *, bool, char **);
extern char* ObfuscateMongoDBString(char *, char **);
extern void EmitAgentTelemetry(char *, char *, double, char *);
extern void SubmitMetric(char *, metric_type_t, char *, double, char **, char *, bool);
extern void SubmitServiceCheck(char *, char *, int, char **, char *, char *);
extern void SubmitEvent(char *, event_t *);
extern void SubmitHistogramBucket(char *, char *, long long, float, float, int, char *, char **, bool);
extern void SubmitEventPlatformEvent(char *, char *, int, char *);
extern void GetSubprocessOutput(char **, char **, char **, char **, int*, char **);
extern char **Tags(char *, int);
extern int IsContainerExcluded(char *, char *, char *);
extern void GetKubeletConnectionInfo(char **);

// _ptr_at is a helper function to get a pointer to an element in an array of strings
char * _ptr_at(char **ptr, int idx) {
    return ptr[idx];
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// LogMessage calls the C function LogMessage
func LogMessage(message string, level int) {
	cMessage := C.CString(message)
	defer C.free(unsafe.Pointer(cMessage))
	C.LogMessage(cMessage, C.int(level))
}

// GetClusterName calls the C function GetClusterName
func GetClusterName() string {
	var cName *C.char
	C.GetClusterName(&cName)
	return C.GoString(cName)
}

// GetConfig calls the C function GetConfig
func GetConfig(key string) string {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	var cValue *C.char
	C.GetConfig(cKey, &cValue)
	return C.GoString(cValue)
}

// GetHostname calls the C function GetHostname
func GetHostname() string {
	var cHostname *C.char
	C.GetHostname(&cHostname)
	return C.GoString(cHostname)
}

// GetHostTags calls the C function GetHostTags
func GetHostTags() string {
	var cTags *C.char
	C.GetHostTags(&cTags)
	return C.GoString(cTags)
}

// GetVersion calls the C function GetVersion
func GetVersion() string {
	var cVersion *C.char
	C.GetVersion(&cVersion)
	return C.GoString(cVersion)
}

// Headers calls the C function Headers
func Headers() string {
	var cHeaders *C.char
	C.Headers(&cHeaders)
	return C.GoString(cHeaders)
}

// ReadPersistentCache calls the C function ReadPersistentCache
func ReadPersistentCache(key string) string {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	return C.GoString(C.ReadPersistentCache(cKey))
}

// SendLog calls the C function SendLog
func SendLog(message, level string) {
	cMessage := C.CString(message)
	cLevel := C.CString(level)
	defer C.free(unsafe.Pointer(cMessage))
	defer C.free(unsafe.Pointer(cLevel))
	C.SendLog(cMessage, cLevel)
}

// SetCheckMetadata calls the C function SetCheckMetadata
func SetCheckMetadata(checkID, key, value string) {
	cCheckID := C.CString(checkID)
	cKey := C.CString(key)
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cCheckID))
	defer C.free(unsafe.Pointer(cKey))
	defer C.free(unsafe.Pointer(cValue))
	C.SetCheckMetadata(cCheckID, cKey, cValue)
}

// SetExternalTags calls the C function SetExternalTags
func SetExternalTags(hostname, sourceType string, tags []string) {
	cHostname := C.CString(hostname)
	cSourceType := C.CString(sourceType)
	cTags := make([]*C.char, len(tags))
	for i, tag := range tags {
		cTags[i] = C.CString(tag)
		defer C.free(unsafe.Pointer(cTags[i]))
	}
	defer C.free(unsafe.Pointer(cHostname))
	defer C.free(unsafe.Pointer(cSourceType))
	C.SetExternalTags(cHostname, cSourceType, &cTags[0])
}

// WritePersistentCache calls the C function WritePersistentCache
func WritePersistentCache(key, value string) {
	cKey := C.CString(key)
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cKey))
	defer C.free(unsafe.Pointer(cValue))
	C.WritePersistentCache(cKey, cValue)
}

// TracemallocEnabled calls the C function TracemallocEnabled
func TracemallocEnabled() bool {
	return bool(C.TracemallocEnabled())
}

// ObfuscateSQL calls the C function ObfuscateSQL
func ObfuscateSQL(sql, dbType string) (string, error) {
	cSQL := C.CString(sql)
	cDbType := C.CString(dbType)
	defer C.free(unsafe.Pointer(cSQL))
	defer C.free(unsafe.Pointer(cDbType))
	var cError *C.char
	result := C.ObfuscateSQL(cSQL, cDbType, &cError)
	if cError != nil {
		return "", errors.New(C.GoString(cError))
	}
	return C.GoString(result), nil
}

// ObfuscateSQLExecPlan calls the C function ObfuscateSQLExecPlan
func ObfuscateSQLExecPlan(plan string, normalize bool) (string, error) {
	cPlan := C.CString(plan)
	defer C.free(unsafe.Pointer(cPlan))
	var cError *C.char
	result := C.ObfuscateSQLExecPlan(cPlan, C.bool(normalize), &cError)
	if cError != nil {
		return "", errors.New(C.GoString(cError))
	}
	return C.GoString(result), nil
}

// ObfuscateMongoDBString calls the C function ObfuscateMongoDBString
func ObfuscateMongoDBString(query string) (string, error) {
	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))
	var cError *C.char
	result := C.ObfuscateMongoDBString(cQuery, &cError)
	if cError != nil {
		return "", errors.New(C.GoString(cError))
	}
	return C.GoString(result), nil
}

// EmitAgentTelemetry calls the C function EmitAgentTelemetry
func EmitAgentTelemetry(name, value string, sampleRate float64, tags string) {
	cName := C.CString(name)
	cValue := C.CString(value)
	cTags := C.CString(tags)
	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cValue))
	defer C.free(unsafe.Pointer(cTags))
	C.EmitAgentTelemetry(cName, cValue, C.double(sampleRate), cTags)
}

// SubmitMetric calls the C function SubmitMetric
func SubmitMetric(name string, metricType C.metric_type_t, metricName string, value float64, tags []string, hostname string, flushFirstValue bool) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	cHostname := C.CString(hostname)
	defer C.free(unsafe.Pointer(cHostname))

	cMetricName := C.CString(metricName)
	defer C.free(unsafe.Pointer(cMetricName))

	cTags := make([]*C.char, len(tags))
	for i, tag := range tags {
		cTags[i] = C.CString(tag)
		defer C.free(unsafe.Pointer(cTags[i]))
	}

	C.SubmitMetric(cName, metricType, cMetricName, C.double(value), &cTags[0], cHostname, C.bool(flushFirstValue))
}

// SubmitServiceCheck calls the C function SubmitServiceCheck
func SubmitServiceCheck(name, scName string, status int, tags []string, hostname, message string) {
	cName := C.CString(name)
	cSCName := C.CString(scName)
	cHostname := C.CString(hostname)
	cMessage := C.CString(message)
	cTags := make([]*C.char, len(tags))
	for i, tag := range tags {
		cTags[i] = C.CString(tag)
		defer C.free(unsafe.Pointer(cTags[i]))
	}
	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cSCName))
	defer C.free(unsafe.Pointer(cHostname))
	defer C.free(unsafe.Pointer(cMessage))
	C.SubmitServiceCheck(cName, cSCName, C.int(status), &cTags[0], cHostname, cMessage)
}

// SubmitEvent calls the C function SubmitEvent
func SubmitEvent(checkID string, event *C.event_t) {
	cCheckID := C.CString(checkID)
	defer C.free(unsafe.Pointer(cCheckID))
	C.SubmitEvent(cCheckID, event)
}

// SubmitHistogramBucket calls the C function SubmitHistogramBucket
func SubmitHistogramBucket(checkID, metricName string, value int64, lowerBound, upperBound float64, monotonic int, hostname string, tags []string, flushFirstValue bool) {
	cCheckID := C.CString(checkID)
	cMetricName := C.CString(metricName)
	cHostname := C.CString(hostname)
	cTags := make([]*C.char, len(tags))
	for i, tag := range tags {
		cTags[i] = C.CString(tag)
		defer C.free(unsafe.Pointer(cTags[i]))
	}
	defer C.free(unsafe.Pointer(cCheckID))
	defer C.free(unsafe.Pointer(cMetricName))
	defer C.free(unsafe.Pointer(cHostname))
	C.SubmitHistogramBucket(cCheckID, cMetricName, C.longlong(value), C.float(lowerBound), C.float(upperBound), C.int(monotonic), cHostname, &cTags[0], C.bool(flushFirstValue))
}

// SubmitEventPlatformEvent calls the C function SubmitEventPlatformEvent
func SubmitEventPlatformEvent(event, eventType string, eventSize int, hostname string) {
	cEvent := C.CString(event)
	cEventType := C.CString(eventType)
	cHostname := C.CString(hostname)
	defer C.free(unsafe.Pointer(cEvent))
	defer C.free(unsafe.Pointer(cEventType))
	defer C.free(unsafe.Pointer(cHostname))
	C.SubmitEventPlatformEvent(cEvent, cEventType, C.int(eventSize), cHostname)
}

// Tags calls the C function Tags
func Tags(tag string, cardinality int) []string {
	cTag := C.CString(tag)
	defer C.free(unsafe.Pointer(cTag))
	cTags := C.Tags(cTag, C.int(cardinality))
	var tags []string
	for i := 0; C._ptr_at(cTags, C.int(i)) != nil; i++ {
		tags = append(tags, C.GoString(C._ptr_at(cTags, C.int(i))))
	}
	return tags
}

// IsContainerExcluded calls the C function IsContainerExcluded
func IsContainerExcluded(containerID, containerName, imageName string) int {
	cContainerID := C.CString(containerID)
	cContainerName := C.CString(containerName)
	cImageName := C.CString(imageName)
	defer C.free(unsafe.Pointer(cContainerID))
	defer C.free(unsafe.Pointer(cContainerName))
	defer C.free(unsafe.Pointer(cImageName))
	return int(C.IsContainerExcluded(cContainerID, cContainerName, cImageName))
}

// GetKubeletConnectionInfo calls the C function GetKubeletConnectionInfo
func GetKubeletConnectionInfo() string {
	var cInfo *C.char
	C.GetKubeletConnectionInfo(&cInfo)
	return C.GoString(cInfo)
}
