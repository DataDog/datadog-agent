// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo

package jmxclient

/*
#cgo LDFLAGS: -L${SRCDIR}/../../../../dev/lib -ljmxclient
#include <stdlib.h>

// graal things

struct __graal_isolate_t;
typedef struct __graal_isolate_t graal_isolate_t;
struct __graal_isolatethread_t;
typedef struct __graal_isolatethread_t graal_isolatethread_t;

typedef unsigned long __graal_uword;

enum { __graal_create_isolate_params_version = 4 };
struct __graal_create_isolate_params_t {
    int version;
    __graal_uword  reserved_address_space_size;
    const char    *auxiliary_image_path;
    __graal_uword  auxiliary_image_reserved_space_size;
    int            _reserved_1;
    char         **_reserved_2;
    int            pkey;
    char           _reserved_3;
    char           _reserved_4;
    char           _reserved_5;
};
typedef struct __graal_create_isolate_params_t graal_create_isolate_params_t;

int graal_create_isolate(graal_create_isolate_params_t* params, graal_isolate_t** isolate, graal_isolatethread_t** thread);

// Forward declarations for JmxClient library functions
int connect_jvm(void*, char*, int);
int prepare_beans(void*, int, char*);
char* collect_beans(void*, int);
int close_jvm(void*, int);
void free_string(void*, char*);
int cleanup_all(void*);
*/
import "C"
import (
	"encoding/json"
	"errors"
	"fmt"
	"unsafe"
)

// JmxClientWrapper wraps the CGo calls to the JmxClient library
type JmxClientWrapper struct {
	isolate unsafe.Pointer
	thread unsafe.Pointer
}

// NewJmxClientWrapper creates a new wrapper instance
func NewJmxClientWrapper() *JmxClientWrapper {
	var isolate *C.graal_isolate_t
	var thread *C.graal_isolatethread_t

	errorCode := C.graal_create_isolate(nil, &isolate, &thread)
	fmt.Println("graal create isolate:", errorCode)
	if errorCode < 0 || isolate == nil || thread == nil {
	    return nil
	}

	return &JmxClientWrapper{
		isolate: unsafe.Pointer(isolate),
		thread:  unsafe.Pointer(thread),
	}
}

// ConnectJVM connects to a JVM instance
// Returns a connection handle (session ID) on success, or an error
func (w *JmxClientWrapper) ConnectJVM(host string, port int) (int, error) {
	cHost := C.CString(host)
	defer C.free(unsafe.Pointer(cHost))

	sessionID := int(C.connect_jvm(w.thread, cHost, C.int(port)))
	if sessionID < 0 {
		return 0, fmt.Errorf("failed to connect to JVM at %s:%d, error code: %d", host, port, sessionID)
	}

	return sessionID, nil
}

// PrepareBeans configures which beans to collect for a given session
// beansConfig should be a JSON string describing the bean collection configuration
func (w *JmxClientWrapper) PrepareBeans(sessionID int, beansConfig string) error {
	cConfig := C.CString(beansConfig)
	defer C.free(unsafe.Pointer(cConfig))

	result := int(C.prepare_beans(w.thread, C.int(sessionID), cConfig))
	if result != 0 {
		return fmt.Errorf("failed to prepare beans for session %d, error code: %d", sessionID, result)
	}

	return nil
}

// CollectBeans collects metrics from configured beans
// Returns a JSON string with the collected metrics
func (w *JmxClientWrapper) CollectBeans(sessionID int) (string, error) {
	cResult := C.collect_beans(w.thread, C.int(sessionID))
	if cResult == nil {
		return "", fmt.Errorf("failed to collect beans for session %d", sessionID)
	}

	result := C.GoString(cResult)
	C.free_string(w.thread, cResult)

	return result, nil
}

// BeanAttribute represents a single attribute name-value pair
type BeanAttribute struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// BeanData represents a collected JMX bean with its attributes
type BeanData struct {
	Path       string          `json:"path"`
	Success    bool            `json:"success"`
	Attributes []BeanAttribute `json:"attributes"`
	Attribute  string          `json:"attribute"`
	Type       string          `json:"type"`
}

// CollectBeansAsMap collects metrics and returns them as a map
// Deprecated: Use CollectBeansAsStructs for proper type-safe unmarshaling
func (w *JmxClientWrapper) CollectBeansAsMap(sessionID int) (map[string]interface{}, error) {
	jsonStr, err := w.CollectBeans(sessionID)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse collected beans JSON: %w", err)
	}

	return result, nil
}

// CollectBeansAsStructs collects metrics and returns them as a slice of BeanData
func (w *JmxClientWrapper) CollectBeansAsStructs(sessionID int) ([]BeanData, error) {
	jsonStr, err := w.CollectBeans(sessionID)
	if err != nil {
		return nil, err
	}

	var result []BeanData
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse collected beans JSON: %w", err)
	}

	return result, nil
}

// CloseJVM closes a specific JVM connection
func (w *JmxClientWrapper) CloseJVM(sessionID int) error {
	result := int(C.close_jvm(w.thread, C.int(sessionID)))
	if result != 0 {
		return fmt.Errorf("failed to close JVM session %d, error code: %d", sessionID, result)
	}

	return nil
}

// CleanupAll cleans up all JVM connections and resources
func (w *JmxClientWrapper) CleanupAll() error {
	result := int(C.cleanup_all(w.thread))
	if result != 0 {
		return errors.New("failed to cleanup all JVM connections")
	}

	return nil
}
